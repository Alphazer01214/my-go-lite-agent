package webui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// uiEnvelopeEvicted tells a page that a newer page took the UI connection.
const uiEnvelopeEvicted = "evicted"

type uiEnvelope struct {
	Version    int                `json:"version"`
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Mount      *MountRef          `json:"mount,omitempty"`
	Kind       string             `json:"kind,omitempty"`
	Command    string             `json:"command,omitempty"`
	Payload    json.RawMessage    `json:"payload,omitempty"`
	Diagnostic *plugin.Diagnostic `json:"diagnostic,omitempty"`
}

type uiConnection struct {
	queue   chan *uiEnvelope
	done    chan struct{}
	once    sync.Once
	evicted atomic.Bool
}

func newUIConnection() *uiConnection {
	return &uiConnection{
		queue: make(chan *uiEnvelope, 64),
		done:  make(chan struct{}),
	}
}

func (c *uiConnection) stop() {
	c.once.Do(func() { close(c.done) })
}

// evict releases the connection to a newer page. The page is told it lost the
// stream so the SDK stops reconnecting, instead of the two pages taking turns
// stealing the single v1 UI connection.
func (c *uiConnection) evict() {
	c.evicted.Store(true)
	c.stop()
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	connection := newUIConnection()
	s.replaceUIConnection(connection)
	defer s.releaseUIConnection(connection)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := http.NewResponseController(w).Flush(); err != nil {
		return
	}

	for {
		select {
		case envelope := <-connection.queue:
			if err := json.NewEncoder(w).Encode(envelope); err != nil {
				return
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				return
			}
		case <-connection.done:
			if connection.evicted.Load() {
				_ = json.NewEncoder(w).Encode(&uiEnvelope{
					Version: protocol.FrameVersion,
					ID:      s.nextUIID(),
					Type:    uiEnvelopeEvicted,
				})
				_ = http.NewResponseController(w).Flush()
			}
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) replaceUIConnection(next *uiConnection) {
	s.mu.Lock()
	previous := s.connection
	s.connection = next
	s.mu.Unlock()
	if previous != nil {
		previous.evict()
	}
}

func (s *Server) releaseUIConnection(connection *uiConnection) {
	s.mu.Lock()
	if s.connection == connection {
		s.connection = nil
	}
	s.mu.Unlock()
	connection.stop()
}

func (s *Server) enqueueUI(envelope *uiEnvelope) bool {
	s.mu.Lock()
	connection := s.connection
	s.mu.Unlock()
	if connection == nil {
		return false
	}
	select {
	case connection.queue <- envelope:
		return true
	case <-connection.done:
		return false
	default:
		return false
	}
}

// PushDiagnostic stores a runtime diagnostic in the snapshot and sends it to
// the active UI connection when one is available.
func (s *Server) PushDiagnostic(diagnostic plugin.Diagnostic) {
	s.mu.Lock()
	s.diagnostics = append(s.diagnostics, diagnostic)
	s.mu.Unlock()

	id := fmt.Sprintf("diag-%d", s.sequence.Add(1))
	s.enqueueUI(&uiEnvelope{
		Version:    protocol.FrameVersion,
		ID:         id,
		Type:       "diagnostic",
		Diagnostic: &diagnostic,
	})
}

// PublishState implements host.UIPublisher.
func (s *Server) PublishState(from string, payload json.RawMessage) (json.RawMessage, error) {
	var input struct {
		MountID string          `json:"mount_id"`
		State   json.RawMessage `json:"state"`
	}
	if err := decodeSingleJSON(payload, &input); err != nil {
		return nil, fmt.Errorf("%w: %v", host.ErrBadPayload, err)
	}
	if input.MountID == "" || len(bytes.TrimSpace(input.State)) == 0 {
		return nil, fmt.Errorf("%w: mount_id and state are required", host.ErrBadPayload)
	}
	mount, err := s.resolveMount(from, input.MountID)
	if err != nil {
		return nil, err
	}
	state := append(json.RawMessage(nil), input.State...)
	s.enqueueUI(&uiEnvelope{
		Version: protocol.FrameVersion,
		ID:      s.nextUIID(),
		Type:    "ui",
		Mount:   mount,
		Kind:    "state",
		Payload: state,
	})
	return json.RawMessage(`{}`), nil
}

// PublishCommand implements host.UIPublisher.
func (s *Server) PublishCommand(from string, payload json.RawMessage) (json.RawMessage, error) {
	var input struct {
		MountID string          `json:"mount_id"`
		Command string          `json:"command"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := decodeSingleJSON(payload, &input); err != nil {
		return nil, fmt.Errorf("%w: %v", host.ErrBadPayload, err)
	}
	if input.MountID == "" || input.Command == "" {
		return nil, fmt.Errorf("%w: mount_id and command are required", host.ErrBadPayload)
	}
	mount, err := s.resolveMount(from, input.MountID)
	if err != nil {
		return nil, err
	}
	commandPayload := append(json.RawMessage(nil), input.Payload...)
	if len(bytes.TrimSpace(commandPayload)) == 0 || bytes.Equal(bytes.TrimSpace(commandPayload), []byte("null")) {
		commandPayload = json.RawMessage(`{}`)
	}
	s.enqueueUI(&uiEnvelope{
		Version: protocol.FrameVersion,
		ID:      s.nextUIID(),
		Type:    "ui",
		Mount:   mount,
		Kind:    "command",
		Command: input.Command,
		Payload: commandPayload,
	})
	return json.RawMessage(`{}`), nil
}

func (s *Server) resolveMount(from, mountID string) (*MountRef, error) {
	key := MountKey{Plugin: from, ID: mountID}
	registry := s.currentRegistry()
	if spec, ok := registry.Lookup(key); ok {
		return &MountRef{Plugin: spec.Key.Plugin, ID: spec.Key.ID}, nil
	}
	for _, spec := range registry.Mounts() {
		if spec.Key.ID == mountID {
			return nil, fmt.Errorf("%w: mount %s is owned by %s", protocol.ErrUIOwnerMismatch, mountID, spec.Key.Plugin)
		}
	}
	return nil, fmt.Errorf("%w: mount %s:%s", protocol.ErrUINotMounted, from, mountID)
}

func (s *Server) nextUIID() string {
	return fmt.Sprintf("ui-push-%d", s.sequence.Add(1))
}

func decodeSingleJSON(payload []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("payload must contain one JSON object")
		}
		return err
	}
	return nil
}
