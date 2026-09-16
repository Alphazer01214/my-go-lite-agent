package serve

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

func (s *Server) routeRequest(from string, f *protocol.Frame) {
	// Host closed: reject star calls without an interceptor plane (ADR-0014).
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		_ = s.writeTo(from, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
			Error: &protocol.FrameError{Code: "host_closed", Message: "host_closed"},
		})
		return
	}

	// Host owns agent/request (log invariant) and agent.inject (append-only notify).
	if f.Cap == AgentCap && (f.Method == "request" || f.Method == "inject" || f.Method == "confirm") {
		s.handleAgentFromPlugin(from, f)
		return
	}

	// Host owns ensurePlugins (ADR-0023).
	if f.Cap == HostCap && f.Method == "ensurePlugins" {
		s.handleEnsurePlugins(from, f)
		return
	}

	// Multi-provider tools (ADR-0018): merge list / route call by tool name.
	if f.Cap == ToolsCap {
		s.routeToolsFromPlugin(from, f)
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	owner, ok := s.provides[f.Cap]
	if !ok || owner == from {
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "capability_unavailable", Message: fmt.Sprintf("unknown capability %q", f.Cap)},
		})
		return
	}
	to := DefaultCallTimeout
	if p := s.plugins[owner]; p != nil {
		to = p.timeout
	}
	s.seq++
	fwdID := fmt.Sprintf("fwd-%d", s.seq)
	s.pending[fwdID] = &wait{
		kind:       waitPlugin,
		caller:     from,
		target:     owner,
		origID:     f.ID,
		cap:        f.Cap,
		method:     f.Method,
		reqPayload: append(json.RawMessage(nil), f.Payload...),
	}
	s.mu.Unlock()

	if err := s.ensureAlive(owner); err != nil {
		s.mu.Lock()
		delete(s.pending, fwdID)
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "route_failed", Message: err.Error()},
		})
		return
	}

	fwd := *f
	fwd.ID = fwdID
	if err := s.writeTo(owner, &fwd); err != nil {
		s.mu.Lock()
		delete(s.pending, fwdID)
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "route_failed", Message: err.Error()},
		})
		return
	}

	timer := time.AfterFunc(to, func() {
		s.mu.Lock()
		w, ok := s.pending[fwdID]
		if ok {
			delete(s.pending, fwdID)
		}
		s.mu.Unlock()
		if !ok {
			return
		}
		_ = s.writeTo(from, &protocol.Frame{
			Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "timeout", Message: fmt.Sprintf("call to %s timed out after %s", owner, to)},
		})
		_ = w
	})
	// timer stopped in complete when pending is removed; leak-once acceptable for lite host if not stopped.
	_ = timer
}

// routeToolsFromPlugin handles star-routed tools.list/call with multi-provider merge (ADR-0018).
func (s *Server) routeToolsFromPlugin(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
	}
	switch f.Method {
	case "list":
		payload, err := s.toolsListMerged()
		if err != nil {
			res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
		} else {
			res.Payload = payload
		}
		_ = s.writeTo(from, res)
	case "call":
		var in struct {
			Name string `json:"name"`
		}
		if len(f.Payload) > 0 {
			_ = json.Unmarshal(f.Payload, &in)
		}
		owner, ok := s.toolsOwnerFor(in.Name)
		if !ok {
			res.Error = &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
			_ = s.writeTo(from, res)
			return
		}
		if owner == from {
			// Self-call of an owned tool: execute via Call (bypass star self-block).
			payload, err := s.callByPlugin(owner, ToolsCap, "call", f.Payload)
			if err != nil {
				res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
			} else {
				res.Payload = payload
			}
			_ = s.writeTo(from, res)
			return
		}
		// Forward like a normal star call to the owning tools Plugin.
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		to := DefaultCallTimeout
		if p := s.plugins[owner]; p != nil {
			to = p.timeout
		}
		s.seq++
		fwdID := fmt.Sprintf("fwd-%d", s.seq)
		s.pending[fwdID] = &wait{
			kind:       waitPlugin,
			caller:     from,
			target:     owner,
			origID:     f.ID,
			cap:        f.Cap,
			method:     f.Method,
			reqPayload: append(json.RawMessage(nil), f.Payload...),
		}
		s.mu.Unlock()
		if err := s.ensureAlive(owner); err != nil {
			s.mu.Lock()
			delete(s.pending, fwdID)
			s.mu.Unlock()
			res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
		fwd := *f
		fwd.ID = fwdID
		if err := s.writeTo(owner, &fwd); err != nil {
			s.mu.Lock()
			delete(s.pending, fwdID)
			s.mu.Unlock()
			res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
		timer := time.AfterFunc(to, func() {
			s.mu.Lock()
			w, ok := s.pending[fwdID]
			if ok {
				delete(s.pending, fwdID)
			}
			s.mu.Unlock()
			if !ok {
				return
			}
			_ = s.writeTo(from, &protocol.Frame{
				Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
				Error: &protocol.FrameError{Code: "timeout", Message: fmt.Sprintf("call to %s timed out after %s", owner, to)},
			})
			_ = w
		})
		_ = timer
	default:
		res.Error = &protocol.FrameError{Code: "method_not_found", Message: "unknown tools." + f.Method}
		_ = s.writeTo(from, res)
	}
}

func (s *Server) complete(f *protocol.Frame) {
	debugf("complete id=%s type=%s cap=%s method=%s", f.ID, f.Type, f.Cap, f.Method)
	s.mu.Lock()
	w, ok := s.pending[f.ID]
	if ok {
		delete(s.pending, f.ID)
	}
	var evts []*protocol.Frame
	if ok && w != nil {
		evts = w.events
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if w.kind == waitHost {
		select {
		case w.ch <- &CallResult{Frame: f, Events: evts}:
		default:
		}
		return
	}
	// No Host-side side effects on plugin-originated calls: session facts are
	// emitted by the Session Plugin itself, and llm usage is pushed by the LLM
	// Plugin via context.noteUsage (ADR-0026: Host does not interpret payloads
	// or duplicate business facts).
	out := *f
	out.ID = w.origID
	_ = s.writeTo(w.caller, &out)
}

func (s *Server) collectEvent(from string, f *protocol.Frame) {
	// Presentation Cards are broadcast (may have no id); record before id filter.
	if f.Cap == PresentationCap && f.Method == PresentationCardMethod {
		s.recordCard(f)
	}
	if f.Cap == PresentationCap && f.Method == PresentationRenderMethod {
		s.dispatchRender(f)
	}
	if f.Cap == PresentationCap && f.Method == PresentationPanelMethod {
		s.dispatchPanel(from, f)
	}
	// Stream signals are relayed as-is: Host does not interpret payload fields
	// (ADR-0026) — the Render Medium parses the public stream contract itself.
	// Both presentation.stream (typed emitter) and the legacy llm.chunk signal
	// are forwarded so external Agents and test stubs stay live.
	if (f.Cap == PresentationCap && f.Method == PresentationStreamMethod) ||
		(f.Cap == LLMCap && f.Method == LLMChunkMethod) {
		s.publish(Event{Topic: "stream", Data: f.Payload})
	}
	if f.ID == "" {
		return
	}
	s.mu.Lock()
	var cb func(*protocol.Frame)
	if w, ok := s.pending[f.ID]; ok {
		w.events = append(w.events, f)
		cb = w.onEvent
	}
	s.mu.Unlock()
	if cb != nil {
		cb(f)
	}
}

func (s *Server) dispatchRender(f *protocol.Frame) {
	// Render intents are a public contract type (pluginsdk.RenderIntent);
	// Host decodes and relays without judging kind or content (ADR-0026:
	// presentation events are emitted by the Plugin, not curated by Host).
	var ri pluginsdk.RenderIntent
	if len(f.Payload) > 0 {
		_ = json.Unmarshal(f.Payload, &ri)
	}
	s.publish(Event{Topic: "presentation", Data: ri})
}

func (s *Server) dispatchPanel(from string, f *protocol.Frame) {
	var op PanelOp
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &op); err != nil {
			s.rejectPanel(from, "malformed payload")
			return
		}
	}
	if err := s.validatePanelOp(from, op); err != nil {
		s.rejectPanel(from, err.Error())
		return
	}
	s.publish(Event{Topic: "panel", Data: op})
}

// validatePanelOp enforces the Panel Component contract: the component tag
// must belong to the emitting plugin (component prefix check authorized by
// ADR-0010 — retained) and payloads must be structurally sound JSON
// (ADR-0026: Host validates structure, not plugin business semantics).
func (s *Server) validatePanelOp(from string, op PanelOp) error {
	if op.Op != "set" && op.Op != "clear" {
		return fmt.Errorf("op %q must be set|clear", op.Op)
	}
	if op.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !plugin.ValidMountSlot(op.Slot) {
		return fmt.Errorf("unknown slot %q", op.Slot)
	}
	if op.Op == "set" {
		if op.Component == "" {
			return fmt.Errorf("component is required for set")
		}
		if !plugin.ValidComponentTag(op.Component) {
			return fmt.Errorf("component %q must be a valid custom element tag", op.Component)
		}
		if !strings.HasPrefix(op.Component, from+"-") {
			return fmt.Errorf("component %q must be prefixed with %q", op.Component, from+"-")
		}
	}
	if len(op.Props) > 0 {
		trimmed := strings.TrimSpace(string(op.Props))
		if !json.Valid(op.Props) || !strings.HasPrefix(trimmed, "{") {
			return fmt.Errorf("props must be a JSON object")
		}
	}
	return nil
}

// rejectPanel suppresses the op and surfaces the violation instead of
// silently dropping it. Panel evt frames carry no id, so there is no
// correlated error Frame; the warn status is the observable rejection.
func (s *Server) rejectPanel(from, reason string) {
	s.publish(Event{Topic: "status", Data: map[string]string{
		"status": fmt.Sprintf("warn: rejected panel op from %s: %s", from, reason),
	}})
}

// handleEnsurePlugins mounts discovered plugins by name (and their dependsOn closure).
func (s *Server) handleEnsurePlugins(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
	}
	var in struct {
		Names []string `json:"names"`
	}
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &in); err != nil {
			res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
	}
	out, err := s.EnsurePlugins(in.Names)
	if err != nil {
		res.Error = &protocol.FrameError{Code: "ensure_plugins_failed", Message: err.Error()}
		_ = s.writeTo(from, res)
		return
	}
	res.Payload = MarshalPayload(out)
	_ = s.writeTo(from, res)
}

// EnsurePluginsResult is the ensurePlugins response (ADR-0023).
type EnsurePluginsResult struct {
	Mounted []string `json:"mounted"`
	Missing []string `json:"missing"`
	Failed  []string `json:"failed,omitempty"`
}

// EnsurePlugins idempotently mounts catalog plugins by name (UI-only allowed).
// The dependsOn closure is expanded by assembly.ResolveClosure (ADR-0021).
func (s *Server) EnsurePlugins(names []string) (*EnsurePluginsResult, error) {
	catalog := s.ensureCatalog()
	out := &EnsurePluginsResult{}

	mountedSet, missing, _ := assembly.ResolveClosure(catalog, names)
	out.Missing = missing

	var toLaunch []discovery.Found
	for _, p := range mountedSet {
		s.mu.Lock()
		_, alive := s.plugins[p.Manifest.Name]
		_, ui := s.mountedUI[p.Manifest.Name]
		s.mu.Unlock()
		if alive || ui {
			out.Mounted = append(out.Mounted, p.Manifest.Name)
			continue
		}
		toLaunch = append(toLaunch, p)
	}

	if len(toLaunch) == 0 {
		return out, nil
	}
	if err := s.registerProvides(toLaunch); err != nil {
		return nil, err
	}
	for _, p := range toLaunch {
		if p.Manifest.Entry == "" {
			s.mu.Lock()
			s.mountedUI[p.Manifest.Name] = true
			s.mu.Unlock()
			out.Mounted = append(out.Mounted, p.Manifest.Name)
			continue
		}
		if err := s.launch(p); err != nil {
			out.Failed = append(out.Failed, p.Manifest.Name)
			fmt.Fprintf(os.Stderr, "ensurePlugins: launch %s: %v\n", p.Manifest.Name, err)
			continue
		}
		out.Mounted = append(out.Mounted, p.Manifest.Name)
	}
	s.reconcileConsumes()
	if err := s.discoverTools(); err != nil {
		fmt.Fprintf(os.Stderr, "ensurePlugins: discoverTools: %v\n", err)
	}
	return out, nil
}

// handleAgentFromPlugin serves Host-owned agent.request and agent.inject.
func (s *Server) handleAgentFromPlugin(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V:      f.V,
		ID:     f.ID,
		Type:   protocol.TypeRes,
		Cap:    f.Cap,
		Method: f.Method,
	}
	switch f.Method {
	case "request":
		var in struct {
			SessionID string    `json:"sessionId"`
			Messages  []Message `json:"messages"`
		}
		if len(f.Payload) > 0 {
			if err := json.Unmarshal(f.Payload, &in); err != nil {
				res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
				_ = s.writeTo(from, res)
				return
			}
		}
		out, err := s.AgentRequest(in.SessionID, in.Messages)
		if err != nil {
			if fe, ok := err.(*protocol.FrameError); ok {
				res.Error = fe
			} else {
				res.Error = &protocol.FrameError{Code: "agent_request_failed", Message: err.Error()}
			}
			_ = s.writeTo(from, res)
			return
		}
		res.Payload = MarshalPayload(out)
		_ = s.writeTo(from, res)
	case "inject":
		out, err := s.AgentInject(f.Payload)
		if err != nil {
			if fe, ok := err.(*protocol.FrameError); ok {
				res.Error = fe
			} else {
				res.Error = &protocol.FrameError{Code: "agent_inject_failed", Message: err.Error()}
			}
			_ = s.writeTo(from, res)
			return
		}
		res.Payload = MarshalPayload(out)
		_ = s.writeTo(from, res)
	case "confirm":
		var in struct {
			Tool        string          `json:"tool"`
			Arguments   json.RawMessage `json:"arguments"`
			Workspace   string          `json:"workspace"`
			SessionID   string          `json:"sessionId"`
			Description string          `json:"description"`
		}
		if len(f.Payload) > 0 {
			if err := json.Unmarshal(f.Payload, &in); err != nil {
				res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
				_ = s.writeTo(from, res)
				return
			}
		}
		approved := s.askApproval(in.Tool, in.Arguments, in.Workspace, in.SessionID)
		res.Payload = MarshalPayload(map[string]any{"approved": approved})
		_ = s.writeTo(from, res)
	default:
		res.Error = &protocol.FrameError{
			Code:    "method_not_found",
			Message: fmt.Sprintf("no handler for agent.%s", f.Method),
		}
		_ = s.writeTo(from, res)
	}
}
