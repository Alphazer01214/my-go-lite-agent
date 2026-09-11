// Package serve keeps mounted Plugins alive and routes Capabilities star-through Host.
package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type waitKind int

const (
	waitHost waitKind = iota
	waitPlugin
)

type wait struct {
	kind   waitKind
	caller string // plugin that originated the req (waitPlugin)
	target string // plugin that should answer
	origID string // caller's frame id (waitPlugin)
	ch     chan *protocol.Frame
}

type proc struct {
	name  string
	cmd   *exec.Cmd
	stdin *os.File
	wmu   sync.Mutex
}

// Server owns Plugin processes and the Capability registry.
type Server struct {
	mu       sync.Mutex
	plugins  map[string]*proc
	provides map[string]string // capability -> plugin name
	pending  map[string]*wait  // frame id currently on the wire toward a target
	closed   bool
	seq      int
}

// Start launches every mounted Plugin and builds the Capability registry from provides.
func Start(mounted []discovery.Found) (*Server, error) {
	s := &Server{
		plugins:  make(map[string]*proc, len(mounted)),
		provides: make(map[string]string),
		pending:  make(map[string]*wait),
	}
	for _, p := range mounted {
		name := p.Manifest.Name
		entry := p.Manifest.ResolveEntry(p.Dir)
		cmd := exec.Command(entry)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("stdin %s: %w", name, err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("stdout %s: %w", name, err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("start %s: %w", name, err)
		}
		s.plugins[name] = &proc{name: name, cmd: cmd, stdin: stdin.(*os.File)}
		for _, capName := range p.Manifest.Provides {
			if owner, ok := s.provides[capName]; ok {
				_ = s.Close()
				return nil, fmt.Errorf("capability %q provided by both %s and %s", capName, owner, name)
			}
			s.provides[capName] = name
		}
		go s.readLoop(name, stdout)
	}
	return s, nil
}

func (s *Server) readLoop(pluginName string, stdout io.Reader) {
	for {
		f, err := protocol.ReadFrame(stdout)
		if err != nil {
			s.dropPlugin(pluginName)
			return
		}
		s.handleFromPlugin(pluginName, f)
	}
}

func (s *Server) dropPlugin(pluginName string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	var hostFail []*wait
	var pluginFail []struct {
		caller string
		origID string
	}
	for id, w := range s.pending {
		if w.target == pluginName {
			delete(s.pending, id)
			if w.kind == waitHost {
				hostFail = append(hostFail, w)
			} else {
				pluginFail = append(pluginFail, struct {
					caller string
					origID string
				}{w.caller, w.origID})
			}
			continue
		}
		if w.kind == waitPlugin && w.caller == pluginName {
			delete(s.pending, id)
		}
	}
	s.mu.Unlock()

	for _, w := range hostFail {
		select {
		case w.ch <- &protocol.Frame{
			Type:  protocol.TypeRes,
			ID:    w.origID,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		}:
		default:
		}
	}
	for _, p := range pluginFail {
		_ = s.writeTo(p.caller, &protocol.Frame{
			Type:  protocol.TypeRes,
			ID:    p.origID,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		})
	}
}

func (s *Server) handleFromPlugin(from string, f *protocol.Frame) {
	switch f.Type {
	case protocol.TypeReq:
		s.routeRequest(from, f)
	case protocol.TypeRes:
		s.complete(f)
	}
}

func (s *Server) routeRequest(from string, f *protocol.Frame) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	owner, ok := s.provides[f.Cap]
	if !ok || owner == from {
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			V:    f.V,
			ID:   f.ID,
			Type: protocol.TypeRes,
			Cap:  f.Cap,
			Error: &protocol.FrameError{
				Code:    "capability_unavailable",
				Message: fmt.Sprintf("unknown capability %q", f.Cap),
			},
		})
		return
	}
	s.seq++
	fwdID := fmt.Sprintf("fwd-%d", s.seq)
	s.pending[fwdID] = &wait{kind: waitPlugin, caller: from, target: owner, origID: f.ID}
	s.mu.Unlock()

	fwd := *f
	fwd.ID = fwdID
	if err := s.writeTo(owner, &fwd); err != nil {
		s.mu.Lock()
		delete(s.pending, fwdID)
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "route_failed", Message: err.Error()},
		})
	}
}

func (s *Server) complete(f *protocol.Frame) {
	s.mu.Lock()
	w, ok := s.pending[f.ID]
	if ok {
		delete(s.pending, f.ID)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if w.kind == waitHost {
		select {
		case w.ch <- f:
		default:
		}
		return
	}
	out := *f
	out.ID = w.origID
	_ = s.writeTo(w.caller, &out)
}

func (s *Server) writeTo(plugin string, f *protocol.Frame) error {
	s.mu.Lock()
	p := s.plugins[plugin]
	closed := s.closed
	s.mu.Unlock()
	if closed || p == nil {
		return fmt.Errorf("plugin %s not connected", plugin)
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	return protocol.WriteFrame(p.stdin, f)
}

// Call sends a Host-initiated req to pluginName and waits for its res.
func (s *Server) Call(pluginName string, f *protocol.Frame) (*protocol.Frame, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("server closed")
	}
	s.seq++
	id := fmt.Sprintf("host-%d", s.seq)
	f.ID = id
	ch := make(chan *protocol.Frame, 1)
	s.pending[id] = &wait{kind: waitHost, target: pluginName, ch: ch}
	s.mu.Unlock()

	if err := s.writeTo(pluginName, f); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}
	return <-ch, nil
}

// Close shuts down every Plugin process.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	plugins := make([]*proc, 0, len(s.plugins))
	for _, p := range s.plugins {
		plugins = append(plugins, p)
	}
	s.mu.Unlock()

	var first error
	for _, p := range plugins {
		p.wmu.Lock()
		_ = p.stdin.Close()
		p.wmu.Unlock()
		if err := p.cmd.Wait(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// MarshalPayload JSON-encodes v for a Call payload.
func MarshalPayload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
