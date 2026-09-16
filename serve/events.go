package serve

import (
	"encoding/json"
	"reflect"
)

// Event is one fan-out payload for every Render Medium (CLI, Web, …).
type Event struct {
	Topic string // presentation | status | stream | panel
	Data  any
}

// Subscriber receives live Render Medium events. Keep handlers fast.
type Subscriber struct {
	OnEvent func(Event)
}

// Subscribe registers a multi-consumer listener. Returns unsubscribe.
func (s *Server) Subscribe(sub *Subscriber) (unsubscribe func()) {
	s.mu.Lock()
	s.subs = append(s.subs, sub)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := s.subs[:0]
		for _, x := range s.subs {
			if x != sub {
				out = append(out, x)
			}
		}
		s.subs = out
	}
}

// RegisterApproval registers a Render Medium face for policy.ask approvals
// (ADR-0029). Multiple faces may be registered concurrently — CLI prompt and
// Web SSE never overwrite each other. Returns unsubscribe.
func RegisterApproval(s *Server, fn func(tool string, arguments json.RawMessage, workspace, sessionID string) bool) func() {
	return s.registerApproval(fn)
}

func (s *Server) registerApproval(fn func(tool string, arguments json.RawMessage, workspace, sessionID string) bool) func() {
	s.mu.Lock()
	s.approvals = append(s.approvals, fn)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := s.approvals[:0]
		for _, x := range s.approvals {
			if x != nil && reflect.ValueOf(x).Pointer() != reflect.ValueOf(fn).Pointer() {
				out = append(out, x)
			}
		}
		s.approvals = out
	}
}

// askApproval asks every registered approval face in parallel; the first
// responder wins (ADR-0029). No faces or all-denies → deny (safe default).
func (s *Server) askApproval(tool string, arguments json.RawMessage, workspace, sessionID string) bool {
	s.mu.Lock()
	fns := append([]func(string, json.RawMessage, string, string) bool(nil), s.approvals...)
	s.mu.Unlock()
	if len(fns) == 0 {
		return false
	}
	if len(fns) == 1 {
		return fns[0](tool, arguments, workspace, sessionID)
	}
	ch := make(chan bool, len(fns))
	for _, fn := range fns {
		go func(f func(string, json.RawMessage, string, string) bool) {
			ch <- f(tool, arguments, workspace, sessionID)
		}(fn)
	}
	return <-ch
}

func (s *Server) publish(e Event) {
	s.mu.Lock()
	subs := make([]*Subscriber, len(s.subs))
	copy(subs, s.subs)
	if e.Topic == "panel" {
		if p, ok := e.Data.(PanelOp); ok {
			s.panels = append(s.panels, p)
			const maxPanels = 256
			if len(s.panels) > maxPanels {
				s.panels = s.panels[len(s.panels)-maxPanels:]
			}
		}
	}
	s.mu.Unlock()
	for _, sub := range subs {
		if sub.OnEvent != nil {
			sub.OnEvent(e)
		}
	}
}

// Panels returns panel ops observed this run (replay seed).
func (s *Server) Panels() []PanelOp {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PanelOp, len(s.panels))
	copy(out, s.panels)
	return out
}
