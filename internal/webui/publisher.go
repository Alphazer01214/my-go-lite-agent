package webui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// maxDiagnostics bounds the runtime diagnostic ring. Diagnostics are replayed in
// every bootstrap, so an unbounded slice would grow the payload forever.
const maxDiagnostics = 256

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
	id    string
	queue chan *uiEnvelope
	drops atomic.Uint64
}

func newUIConnection() *uiConnection {
	return &uiConnection{
		queue: make(chan *uiEnvelope, 64),
	}
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	connection := newUIConnection()
	connection.id = fmt.Sprintf("ui-conn-%d", s.connectionSeq.Add(1))
	s.addUIConnection(connection)
	defer s.removeUIConnection(connection)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := http.NewResponseController(w).Flush(); err != nil {
		return
	}
	// hello is per connection and always precedes queued pushes. The SDK uses
	// every hello after the first one as the signal to resync page state.
	hello := &uiEnvelope{
		Version: protocol.FrameVersion,
		ID:      s.nextUIID(),
		Type:    "hello",
	}
	if err := json.NewEncoder(w).Encode(hello); err != nil {
		return
	}
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
		case <-r.Context().Done():
			return
		}
	}
}

// addUIConnection publishes one more UI stream. Connections are independent:
// a new page never displaces an existing one.
func (s *Server) addUIConnection(connection *uiConnection) {
	s.mu.Lock()
	if s.connections == nil {
		s.connections = make(map[*uiConnection]struct{})
	}
	s.connections[connection] = struct{}{}
	total := len(s.connections)
	s.mu.Unlock()
	s.log("ui: connection opened id=%s total=%d", connection.id, total)
}

func (s *Server) removeUIConnection(connection *uiConnection) {
	s.mu.Lock()
	delete(s.connections, connection)
	total := len(s.connections)
	s.mu.Unlock()
	s.log("ui: connection closed id=%s total=%d drops=%d",
		connection.id, total, connection.drops.Load())
}

// enqueueUI fans one envelope out to every active connection. A slow or dead
// page only loses its own queue; delivery succeeds when at least one page took
// the envelope.
func (s *Server) enqueueUI(envelope *uiEnvelope) bool {
	s.mu.Lock()
	connections := make([]*uiConnection, 0, len(s.connections))
	for connection := range s.connections {
		connections = append(connections, connection)
	}
	s.mu.Unlock()

	delivered := false
	for _, connection := range connections {
		select {
		case connection.queue <- envelope:
			delivered = true
		default:
			if connection.drops.Add(1) == 1 {
				s.log("ui: queue full id=%s", connection.id)
			}
		}
	}
	return delivered
}

// PushDiagnostic stores a runtime diagnostic in the snapshot and sends it to
// the active UI connection when one is available.
func (s *Server) PushDiagnostic(diagnostic plugin.Diagnostic) {
	s.mu.Lock()
	s.diagnostics = append(s.diagnostics, diagnostic)
	if len(s.diagnostics) > maxDiagnostics {
		s.diagnostics = s.diagnostics[len(s.diagnostics)-maxDiagnostics:]
	}
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
	if !s.enqueueUI(&uiEnvelope{
		Version: protocol.FrameVersion,
		ID:      s.nextUIID(),
		Type:    "ui",
		Mount:   mount,
		Kind:    "state",
		Payload: state,
	}) {
		// No browser attached or the queue is saturated. The plugin is told so
		// instead of being handed a success it cannot observe.
		return nil, fmt.Errorf("%w: no ui connection for mount %s", protocol.ErrUINotMounted, input.MountID)
	}
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
	if !s.enqueueUI(&uiEnvelope{
		Version: protocol.FrameVersion,
		ID:      s.nextUIID(),
		Type:    "ui",
		Mount:   mount,
		Kind:    "command",
		Command: input.Command,
		Payload: commandPayload,
	}) {
		return nil, fmt.Errorf("%w: no ui connection for mount %s", protocol.ErrUINotMounted, input.MountID)
	}
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
