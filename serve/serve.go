// Package serve keeps mounted Plugins alive and routes Capabilities star-through Host.
package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// DefaultCallTimeout is used when a Plugin manifest omits timeoutMs.
const DefaultCallTimeout = 30 * time.Second

// DefaultShutdownGrace is how long Close waits after stdin EOF before killing.
const DefaultShutdownGrace = 2 * time.Second

type waitKind int

const (
	waitHost waitKind = iota
	waitPlugin
)

type wait struct {
	kind   waitKind
	caller string
	target string
	origID string
	ch     chan *CallResult
	events []*protocol.Frame
}

// CallResult carries one Call's final res plus any evt frames collected while waiting.
type CallResult struct {
	Frame  *protocol.Frame
	Events []*protocol.Frame
}

type proc struct {
	found   discovery.Found
	cmd     *exec.Cmd
	stdin   *os.File
	wmu     sync.Mutex
	healthy bool
	timeout time.Duration
	gen     int
}

// Server owns Plugin processes and the Capability registry.
type Server struct {
	mu       sync.Mutex
	plugins  map[string]*proc
	provides map[string]string
	pending  map[string]*wait
	closed   bool
	seq      int
	gen      map[string]int
}

// Start launches every mounted Plugin, checks consumes, and builds the Capability registry.
func Start(mounted []discovery.Found) (*Server, error) {
	s := &Server{
		plugins:  make(map[string]*proc, len(mounted)),
		provides: make(map[string]string),
		pending:  make(map[string]*wait),
		gen:      make(map[string]int),
	}
	for _, p := range mounted {
		for _, capName := range p.Manifest.Provides {
			if owner, ok := s.provides[capName]; ok {
				_ = s.Close()
				return nil, fmt.Errorf("capability %q provided by both %s and %s", capName, owner, p.Manifest.Name)
			}
			s.provides[capName] = p.Manifest.Name
		}
	}
	for _, p := range mounted {
		if err := s.launch(p); err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	if err := s.checkConsumes(mounted); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Server) checkConsumes(mounted []discovery.Found) error {
	for _, p := range mounted {
		for _, need := range p.Manifest.Consumes {
			if _, ok := s.provides[need]; !ok {
				return fmt.Errorf("plugin %s consumes %q but no mounted plugin provides it", p.Manifest.Name, need)
			}
		}
	}
	return nil
}

func (s *Server) launch(found discovery.Found) error {
	name := found.Manifest.Name
	entry := found.Manifest.ResolveEntry(found.Dir)
	cmd := exec.Command(entry)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin %s: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout %s: %w", name, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	to := DefaultCallTimeout
	if found.Manifest.TimeoutMs > 0 {
		to = time.Duration(found.Manifest.TimeoutMs) * time.Millisecond
	}
	s.gen[name]++
	g := s.gen[name]
	s.plugins[name] = &proc{
		found:   found,
		cmd:     cmd,
		stdin:   stdin.(*os.File),
		healthy: true,
		timeout: to,
		gen:     g,
	}
	go s.readLoop(name, g, stdout)
	return nil
}

func (s *Server) readLoop(pluginName string, gen int, stdout io.Reader) {
	for {
		f, err := protocol.ReadFrame(stdout)
		if err != nil {
			s.markUnhealthy(pluginName, gen)
			return
		}
		s.handleFromPlugin(pluginName, f)
	}
}

func (s *Server) markUnhealthy(pluginName string, gen int) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	p := s.plugins[pluginName]
	if p == nil || p.gen != gen {
		s.mu.Unlock()
		return
	}
	p.healthy = false
	var hostFail []*wait
	var pluginFail []struct{ caller, origID string }
	for id, w := range s.pending {
		if w.target == pluginName {
			delete(s.pending, id)
			if w.kind == waitHost {
				hostFail = append(hostFail, w)
			} else {
				pluginFail = append(pluginFail, struct{ caller, origID string }{w.caller, w.origID})
			}
		}
	}
	s.mu.Unlock()

	for _, w := range hostFail {
		select {
		case w.ch <- &CallResult{Frame: &protocol.Frame{
			Type:  protocol.TypeRes,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		}}:
		default:
		}
	}
	for _, pf := range pluginFail {
		_ = s.writeTo(pf.caller, &protocol.Frame{
			Type:  protocol.TypeRes,
			ID:    pf.origID,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		})
	}
}

func (s *Server) ensureAlive(name string) error {
	s.mu.Lock()
	p := s.plugins[name]
	if p == nil {
		s.mu.Unlock()
		return fmt.Errorf("plugin %s not mounted", name)
	}
	if p.healthy {
		s.mu.Unlock()
		return nil
	}
	found := p.found
	old := p.cmd
	s.mu.Unlock()

	// Reap old process without hanging the caller.
	if old != nil && old.Process != nil {
		_ = old.Process.Kill()
		_, _ = old.Process.Wait()
	}
	s.mu.Lock()
	// Another goroutine may have restarted already.
	if cur := s.plugins[name]; cur != nil && cur.healthy {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	return s.launch(found)
}

func (s *Server) handleFromPlugin(from string, f *protocol.Frame) {
	switch f.Type {
	case protocol.TypeReq:
		s.routeRequest(from, f)
	case protocol.TypeRes:
		s.complete(f)
	case protocol.TypeEvt:
		s.collectEvent(f)
	}
}

func (s *Server) collectEvent(f *protocol.Frame) {
	if f.ID == "" {
		return
	}
	s.mu.Lock()
	if w, ok := s.pending[f.ID]; ok {
		w.events = append(w.events, f)
	}
	s.mu.Unlock()
}

func (s *Server) routeRequest(from string, f *protocol.Frame) {
	// Host owns agent/request (log invariant). Other agent.* methods may be Plugin-provided.
	if f.Cap == AgentCap && f.Method == "request" {
		s.handleAgentFromPlugin(from, f)
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
	s.pending[fwdID] = &wait{kind: waitPlugin, caller: from, target: owner, origID: f.ID}
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

func (s *Server) complete(f *protocol.Frame) {
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

// Call sends a Host-initiated req to pluginName and waits for its res (with timeout).
// Retries once on plugin_down after on-demand restart (crash recovery).
func (s *Server) Call(pluginName string, f *protocol.Frame) (*protocol.Frame, error) {
	out, err := s.CallStream(pluginName, f)
	if err != nil {
		return nil, err
	}
	return out.Frame, nil
}

// CallStream is Call plus any evt frames attributed to the request id while waiting.
func (s *Server) CallStream(pluginName string, f *protocol.Frame) (*CallResult, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			if err := s.ensureAlive(pluginName); err != nil {
				return nil, lastErr
			}
		} else if err := s.ensureAlive(pluginName); err != nil {
			return nil, err
		}
		res, err := s.callOnce(pluginName, f)
		if err != nil {
			lastErr = err
			continue
		}
		if res.Frame.Error != nil && res.Frame.Error.Code == "plugin_down" && attempt == 0 {
			lastErr = fmt.Errorf("%s", res.Frame.Error.Error())
			continue
		}
		return res, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("call failed")
	}
	return nil, lastErr
}

func (s *Server) callOnce(pluginName string, f *protocol.Frame) (*CallResult, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("server closed")
	}
	to := DefaultCallTimeout
	if p := s.plugins[pluginName]; p != nil {
		to = p.timeout
	}
	s.seq++
	id := fmt.Sprintf("host-%d", s.seq)
	f.ID = id
	ch := make(chan *CallResult, 1)
	s.pending[id] = &wait{kind: waitHost, target: pluginName, ch: ch}
	s.mu.Unlock()

	if err := s.writeTo(pluginName, f); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(to):
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("timeout: call to %s timed out after %s", pluginName, to)
	}
}

// Close shuts down every Plugin: stdin EOF, grace wait, then kill.
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

	for _, p := range plugins {
		p.wmu.Lock()
		_ = p.stdin.Close()
		p.wmu.Unlock()
	}
	deadline := time.Now().Add(DefaultShutdownGrace)
	for _, p := range plugins {
		done := make(chan error, 1)
		go func(cmd *exec.Cmd) { done <- cmd.Wait() }(p.cmd)
		remain := time.Until(deadline)
		if remain < 0 {
			remain = 0
		}
		select {
		case <-done:
		case <-time.After(remain):
			_ = p.cmd.Process.Kill()
			<-done
		}
	}
	return nil
}

// MarshalPayload JSON-encodes v for a Call payload.
func MarshalPayload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// Message is one model-visible chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SessionCap is the Capability name Session Plugins must provide.
const SessionCap = "session"

// AgentCap is the Host-owned Capability namespace for agent/request (and later agent.inject).
const AgentCap = "agent"

// LLMCap is the Capability name LLM Plugins must provide for the default Loop.
const LLMCap = "llm"

// TurnResult is one default-Loop turn (ADR-0003: Loop compiled into Host).
type TurnResult struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Chunks    []string  `json:"chunks"`
	Messages  []Message `json:"messages"`
}

// AgentRequestResult is the validated Model Context after the log invariant check.
type AgentRequestResult struct {
	Messages []Message `json:"messages"`
	// Rebuilt is true when Host rebuilt Model Context from the log (empty claimed).
	// When claimed was supplied and matched, Rebuilt is false (validated, not rebuilt).
	Rebuilt bool `json:"rebuilt"`
}

// QuerySessionFacts lists Session Log facts via session.query.
func (s *Server) QuerySessionFacts(afterSeq, limit int) ([]map[string]any, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "query",
		Payload: MarshalPayload(map[string]int{"afterSeq": afterSeq, "limit": limit}),
	})
	if err != nil {
		return nil, fmt.Errorf("session.query: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("session.query: %w", res.Error)
	}
	var out struct {
		Facts []map[string]any `json:"facts"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil {
		return nil, fmt.Errorf("session.query: bad payload: %w", err)
	}
	if out.Facts == nil {
		out.Facts = []map[string]any{}
	}
	return out.Facts, nil
}

// DeriveMessages rebuilds Model Context from the mounted Session Plugin (session.derive).
func (s *Server) DeriveMessages() ([]Message, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "derive",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		return nil, fmt.Errorf("session.derive: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("session.derive: %w", res.Error)
	}
	var out struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil {
		return nil, fmt.Errorf("session.derive: bad payload: %w", err)
	}
	if out.Messages == nil {
		out.Messages = []Message{}
	}
	return out.Messages, nil
}

// AppendSessionFacts appends each fact to the mounted Session Plugin.
func (s *Server) AppendSessionFacts(facts []map[string]any) (int, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	last := 0
	for _, fact := range facts {
		res, err := s.Call(owner, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     SessionCap,
			Method:  "append",
			Payload: MarshalPayload(fact),
		})
		if err != nil {
			return last, fmt.Errorf("session.append: %w", err)
		}
		if res.Error != nil {
			return last, fmt.Errorf("session.append: %w", res.Error)
		}
		var out struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return last, fmt.Errorf("session.append: bad payload: %w", err)
		}
		last = out.Seq
	}
	return last, nil
}

// AgentRequest enforces the Session Log invariant before a model call (ADR-0002).
//
// claimed empty/nil → rebuild from session.derive and accept.
// claimed non-empty → must match session.derive exactly, else session_invariant_violation.
func (s *Server) AgentRequest(claimed []Message) (*AgentRequestResult, error) {
	derived, err := s.DeriveMessages()
	if err != nil {
		return nil, fmt.Errorf("agent/request: %w", err)
	}
	if len(claimed) > 0 && !messagesEqual(claimed, derived) {
		return nil, &protocol.FrameError{
			Code:    "session_invariant_violation",
			Message: "model context is not reconstructable from session log",
		}
	}
	return &AgentRequestResult{Messages: derived, Rebuilt: len(claimed) == 0}, nil
}

// RunTurn is the Host-compiled default Agent Loop for one chat turn (no tools).
//
// Flow: session.append(user) → AgentRequest(rebuild) → llm.complete(stream) → session.append(assistant).
func (s *Server) RunTurn(userInput string) (*TurnResult, error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, fmt.Errorf("agent loop: user input is required")
	}

	if _, err := s.AppendSessionFacts([]map[string]any{{
		"type":    "message",
		"role":    "user",
		"content": userInput,
	}}); err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	// Invariant: Model Context must be rebuildable from Session Log (ADR-0002).
	ar, err := s.AgentRequest(nil)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	s.mu.Lock()
	llmOwner, ok := s.provides[LLMCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("agent loop: no plugin provides %q", LLMCap)
	}

	payload := MarshalPayload(map[string]any{"messages": ar.Messages})
	out, err := s.CallStream(llmOwner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     LLMCap,
		Method:  "complete",
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("agent loop: llm.complete: %w", err)
	}
	if out.Frame.Error != nil {
		return nil, fmt.Errorf("agent loop: llm.complete: %w", out.Frame.Error)
	}

	var llmOut struct {
		Content string `json:"content"`
	}
	if len(out.Frame.Payload) > 0 {
		if err := json.Unmarshal(out.Frame.Payload, &llmOut); err != nil {
			return nil, fmt.Errorf("agent loop: llm.complete: bad payload: %w", err)
		}
	}

	var chunks []string
	for _, ev := range out.Events {
		if ev.Method != "chunk" {
			continue
		}
		var c struct {
			Delta string `json:"delta"`
		}
		if len(ev.Payload) > 0 {
			_ = json.Unmarshal(ev.Payload, &c)
		}
		chunks = append(chunks, c.Delta)
	}

	if _, err := s.AppendSessionFacts([]map[string]any{{
		"type":    "message",
		"role":    "assistant",
		"content": llmOut.Content,
	}}); err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	msgs, err := s.DeriveMessages()
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	return &TurnResult{
		User:      userInput,
		Assistant: llmOut.Content,
		Chunks:    chunks,
		Messages:  msgs,
	}, nil
}

func messagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
	}
	return true
}

// handleAgentFromPlugin serves Host-owned agent.request so Plugins cannot bypass the log invariant.
func (s *Server) handleAgentFromPlugin(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V:      f.V,
		ID:     f.ID,
		Type:   protocol.TypeRes,
		Cap:    f.Cap,
		Method: f.Method,
	}
	var in struct {
		Messages []Message `json:"messages"`
	}
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &in); err != nil {
			res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
	}
	out, err := s.AgentRequest(in.Messages)
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
}
