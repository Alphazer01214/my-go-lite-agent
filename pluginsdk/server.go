// Package pluginsdk is the Plugin author API: Serve/Handle/Call/Emit over Host Frames.
//
// Public contract (breaking changes require protocol field bump):
//   - Frame: uint32 big-endian length + JSON; fields v,id,type,cap,method,payload,error
//   - Manifest: plugin.json beside the executable; protocol must equal 1
//
// Native and third-party Plugins use this package; do not hand-roll Frame loops.
package pluginsdk

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// Request is one inbound Host req delivered to a Handler.
type Request struct {
	ID      string
	Cap     string
	Method  string
	Payload json.RawMessage
}

// Handler processes one Call Payload and returns a result body (or error).
type Handler func(req *Request) (json.RawMessage, error)

// Server is the in-Plugin runtime: dispatches req, completes outbound Call, emits evt.
type Server struct {
	mu       sync.Mutex
	handlers map[string]Handler
	pending  map[string]chan *protocol.Frame
	out      atomic.Int64
	stdin    *os.File
	stdout   *os.File
}

// New returns an empty Server bound to process stdio.
func New() *Server {
	return &Server{
		handlers: make(map[string]Handler),
		pending:  make(map[string]chan *protocol.Frame),
		stdin:    os.Stdin,
		stdout:   os.Stdout,
	}
}

// Handle registers cap.method. Last registration wins.
func (s *Server) Handle(cap, method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[cap+"."+method] = h
}

// Emit sends an evt Frame (no response expected).
func (s *Server) Emit(cap, method string, payload json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protocol.WriteFrame(s.stdout, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeEvt,
		Cap:     cap,
		Method:  method,
		Payload: payload,
	})
}

// EmitTo sends an evt Frame tagged with a request id so Host can attribute it to a Call.
func (s *Server) EmitTo(id, cap, method string, payload json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protocol.WriteFrame(s.stdout, &protocol.Frame{
		V:       protocol.Version,
		ID:      id,
		Type:    protocol.TypeEvt,
		Cap:     cap,
		Method:  method,
		Payload: payload,
	})
}

// Call invokes another Capability through Host (star topology). Blocks until res or error.
func (s *Server) Call(cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	id := fmt.Sprintf("sdk-%d", s.out.Add(1))
	ch := make(chan *protocol.Frame, 1)
	s.mu.Lock()
	s.pending[id] = ch
	err := protocol.WriteFrame(s.stdout, &protocol.Frame{
		V:       protocol.Version,
		ID:      id,
		Type:    protocol.TypeReq,
		Cap:     cap,
		Method:  method,
		Payload: payload,
	})
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("write call %s.%s: %w", cap, method, err)
	}
	res := <-ch
	if res == nil {
		return nil, fmt.Errorf("call %s.%s: connection closed", cap, method)
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Payload, nil
}

// Serve runs the Frame loop until stdin closes. Safe for concurrent Call/Handle.
func (s *Server) Serve() error {
	for {
		f, err := protocol.ReadFrame(s.stdin)
		if err != nil {
			s.failPending()
			return nil
		}
		switch f.Type {
		case protocol.TypeReq:
			go s.dispatch(f)
		case protocol.TypeRes:
			s.complete(f)
		default:
			// ignore unknown types for forward compatibility
		}
	}
}

func (s *Server) failPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
}

func (s *Server) complete(f *protocol.Frame) {
	s.mu.Lock()
	ch, ok := s.pending[f.ID]
	if ok {
		delete(s.pending, f.ID)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	ch <- f
}

func (s *Server) dispatch(reqFrame *protocol.Frame) {
	key := reqFrame.Cap + "." + reqFrame.Method
	s.mu.Lock()
	h, ok := s.handlers[key]
	s.mu.Unlock()

	res := &protocol.Frame{
		V:      protocol.Version,
		ID:     reqFrame.ID,
		Type:   protocol.TypeRes,
		Cap:    reqFrame.Cap,
		Method: reqFrame.Method,
	}
	if !ok {
		res.Error = &protocol.FrameError{
			Code:    "method_not_found",
			Message: fmt.Sprintf("no handler for %s", key),
		}
	} else {
		out, err := h(&Request{
			ID:      reqFrame.ID,
			Cap:     reqFrame.Cap,
			Method:  reqFrame.Method,
			Payload: reqFrame.Payload,
		})
		if err != nil {
			if fe, ok := err.(*protocol.FrameError); ok {
				res.Error = fe
			} else {
				res.Error = &protocol.FrameError{Code: "handler_error", Message: err.Error()}
			}
		} else {
			res.Payload = out
		}
	}
	s.mu.Lock()
	_ = protocol.WriteFrame(s.stdout, res)
	s.mu.Unlock()
}
