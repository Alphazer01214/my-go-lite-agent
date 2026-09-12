package serve

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
