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
	"sync/atomic"
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
	kind    waitKind
	caller  string
	target  string
	origID  string
	ch      chan *CallResult
	events  []*protocol.Frame
	onEvent func(*protocol.Frame)
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
	audit    []AuditEntry
	cards    []PresentationCard
	panels   []PanelOp
	subs     []*Subscriber
	turnMu   sync.Mutex
	turnCancel atomic.Bool
	job      *jobHolder
	// OnStreamDelta is the live Render Medium hook for ephemeral stream chunks.
	OnStreamDelta func(delta string)
	// OnStatus is the live Render Medium hook for agent idle/running.
	OnStatus func(status string)
	// OnToolCall is the live Render Medium hook when the Loop starts a tool (or Subagent).
	OnToolCall func(name string, arguments json.RawMessage)
	// OnRender is the live Render Medium hook for presentation.render intents.
	OnRender func(ri RenderIntent)
}

// PanelOp is one Web Medium panel mutation (ADR-0009).
type PanelOp struct {
	Op   string `json:"op"`
	Slot string `json:"slot"`
	ID   string `json:"id"`
	HTML string `json:"html,omitempty"`
}

// PresentationPanelMethod is a Panel injection op (set|append|clear).
const PresentationPanelMethod = "panel"

// UICap is the Capability for UI Action routing from the Web Shell.
const UICap = "ui"

// UIActionMethod is the method Plugins implement to handle UI Actions.
const UIActionMethod = "action"

// PresentationCap is the Capability used for Presentation Card evt frames.
const PresentationCap = "presentation"

// PresentationCardMethod is the evt method for a Presentation Card.
const PresentationCardMethod = "card"

// PresentationStreamMethod is the ephemeral stream evt (start/chunk/end); not logged.
const PresentationStreamMethod = "stream"

// PresentationStatusMethod is the agent status evt (idle/running) for Render Medium.
const PresentationStatusMethod = "status"

// PresentationRenderMethod is a classified render intent (markdown_text|message_text|summary_text).
const PresentationRenderMethod = "render"

// CommandsCap is the Capability Host uses to route slash commands into a Plugin.
const CommandsCap = "commands"

// CommandsCallMethod is the method Plugins implement to handle a slash command.
const CommandsCallMethod = "call"

// PresentationCard is a structured UI render intent projected from args/result (no I/O).
type PresentationCard struct {
	CardType string          `json:"cardType"`
	Tool     string          `json:"tool,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// Cards returns a copy of Presentation Cards observed this run.
func (s *Server) Cards() []PresentationCard {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PresentationCard, len(s.cards))
	copy(out, s.cards)
	return out
}

func (s *Server) recordCard(f *protocol.Frame) {
	var c PresentationCard
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &c); err != nil {
			return
		}
	}
	if c.CardType == "" {
		return
	}
	s.mu.Lock()
	s.cards = append(s.cards, c)
	s.mu.Unlock()
}

// Start launches every mounted Plugin, checks consumes, and builds the Capability registry.
func Start(mounted []discovery.Found) (*Server, error) {
	s := &Server{
		plugins:  make(map[string]*proc, len(mounted)),
		provides: make(map[string]string),
		pending:  make(map[string]*wait),
		gen:      make(map[string]int),
	}
	job, err := newJob()
	if err != nil {
		// Non-fatal: fall back to explicit Close/killTree only.
		fmt.Fprintf(os.Stderr, "warn: job object unavailable: %v\n", err)
	} else {
		s.job = job
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
		_ = stdin.Close()
		return fmt.Errorf("stdout %s: %w", name, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}
	// Bind child to Host lifetime (Windows Job Object: kill-on-parent-close).
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()
	if err := job.assign(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "warn: assign %s to job: %v\n", name, err)
	}
	to := DefaultCallTimeout
	if found.Manifest.TimeoutMs > 0 {
		to = time.Duration(found.Manifest.TimeoutMs) * time.Millisecond
	}
	s.mu.Lock()
	s.gen[name]++
	g := s.gen[name]
	if old, ok := s.plugins[name]; ok && old != nil && old.stdin != nil {
		old.wmu.Lock()
		_ = old.stdin.Close()
		old.wmu.Unlock()
	}
	s.plugins[name] = &proc{
		found:   found,
		cmd:     cmd,
		stdin:   stdin.(*os.File),
		healthy: true,
		timeout: to,
		gen:     g,
	}
	s.mu.Unlock()
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

	// Reap old process without hanging the caller. Do not Wait here:
	// Close owns Wait; a second Wait on the same Cmd panics.
	if old != nil {
		s.mu.Lock()
		// best-effort stdin close if we still hold it
		s.mu.Unlock()
		killTree(old)
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
	// Presentation Cards are broadcast (may have no id); record before id filter.
	if f.Cap == PresentationCap && f.Method == PresentationCardMethod {
		s.recordCard(f)
	}
	if f.Cap == PresentationCap && f.Method == PresentationRenderMethod {
		s.dispatchRender(f)
	}
	if f.Cap == PresentationCap && f.Method == PresentationPanelMethod {
		s.dispatchPanel(f)
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
	var ri struct {
		Kind   string        `json:"kind"`
		Text   string        `json:"text"`
		Level  string        `json:"level"`
		Title  string        `json:"title"`
		Pairs  []SummaryPair `json:"pairs"`
		Detail string        `json:"detail"`
	}
	if len(f.Payload) > 0 {
		_ = json.Unmarshal(f.Payload, &ri)
	}
	intent := RenderIntent{
		Kind:   ri.Kind,
		Text:   ri.Text,
		Level:  ri.Level,
		Title:  ri.Title,
		Pairs:  ri.Pairs,
		Detail: ri.Detail,
	}
	switch ri.Kind {
	case KindMarkdownText, KindMessageText, KindSummaryText:
		if s.OnRender != nil {
			s.OnRender(intent)
		}
		s.publish(Event{Topic: "presentation", Data: intent})
	default:
		if s.OnStatus != nil {
			s.OnStatus(fmt.Sprintf("warn: dropped unknown render kind %q", ri.Kind))
		}
	}
}

func (s *Server) dispatchPanel(f *protocol.Frame) {
	var op PanelOp
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &op); err != nil {
			return
		}
	}
	if op.ID == "" {
		return
	}
	s.publish(Event{Topic: "panel", Data: op})
}

func (s *Server) routeRequest(from string, f *protocol.Frame) {
	// Dual Waterfall first: built-in cancel/audit always on; optional external Interceptor.
	// Applies to every plugin-originated star call, including agent/request.
	dec := s.runWaterfall(from, f)
	if dec.Action == ActionReject {
		code := dec.Code
		if code == "" {
			code = "interceptor_rejected"
		}
		_ = s.writeTo(from, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
			Error: &protocol.FrameError{Code: code, Message: dec.Reason},
		})
		return
	}
	if dec.Action == ActionRewrite && len(dec.Payload) > 0 {
		f.Payload = dec.Payload
	}

	// Host owns agent/request (log invariant) and agent.inject (append-only notify).
	if f.Cap == AgentCap && (f.Method == "request" || f.Method == "inject") {
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

// CallCommand routes a slash command into pluginName (cap=commands, method=call).
func (s *Server) CallCommand(pluginName, command, args string) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]string{
		"command": command,
		"args":    args,
	})
	if err != nil {
		return nil, err
	}
	out, err := s.Call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     CommandsCap,
		Method:  CommandsCallMethod,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Payload, nil
}

// CallUIAction routes a UI Action into pluginName (cap=ui, method=action).
func (s *Server) CallUIAction(pluginName string, payload json.RawMessage) (json.RawMessage, error) {
	out, err := s.Call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     UICap,
		Method:  UIActionMethod,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Payload, nil
}

// CallByCap routes cap.method to the Plugin that provides cap.
func (s *Server) CallByCap(cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	owner, ok := s.provides[cap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", cap)
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	out, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     cap,
		Method:  method,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Payload, nil
}

// CallStream is Call plus any evt frames attributed to the request id while waiting.
func (s *Server) CallStream(pluginName string, f *protocol.Frame) (*CallResult, error) {
	return s.CallStreamOn(pluginName, f, nil)
}

// CallStreamOn is CallStream with a live callback for each attributed evt (Render Medium).
// Callback runs on the Host read-loop goroutine; keep it fast and non-blocking.
func (s *Server) CallStreamOn(pluginName string, f *protocol.Frame, onEvent func(*protocol.Frame)) (*CallResult, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			if err := s.ensureAlive(pluginName); err != nil {
				return nil, lastErr
			}
		} else if err := s.ensureAlive(pluginName); err != nil {
			return nil, err
		}
		res, err := s.callOnce(pluginName, f, onEvent)
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

func (s *Server) callOnce(pluginName string, f *protocol.Frame, onEvent func(*protocol.Frame)) (*CallResult, error) {
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
	s.pending[id] = &wait{kind: waitHost, target: pluginName, ch: ch, onEvent: onEvent}
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

// Close shuts down every Plugin: stdin EOF, grace wait, then kill tree.
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
	job := s.job
	s.job = nil
	s.mu.Unlock()

	for _, p := range plugins {
		p.wmu.Lock()
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		p.wmu.Unlock()
	}
	deadline := time.Now().Add(DefaultShutdownGrace)
	for _, p := range plugins {
		if p.cmd == nil || p.cmd.Process == nil {
			continue
		}
		done := make(chan error, 1)
		go func(cmd *exec.Cmd) { done <- cmd.Wait() }(p.cmd)
		remain := time.Until(deadline)
		if remain < 0 {
			remain = 0
		}
		select {
		case <-done:
		case <-time.After(remain):
			killTree(p.cmd)
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}
	// Closing the Job Object (KILL_ON_JOB_CLOSE) reaps any stragglers,
	// including when Host itself is dying without a clean Close of each child.
	if job != nil {
		job.close()
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
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// SessionCap is the Capability name Session Plugins must provide.
const SessionCap = "session"

// AgentCap is the Host-owned Capability namespace for agent/request and agent.inject.
const AgentCap = "agent"

// LLMCap is the Capability name LLM Plugins must provide for the default Loop.
const LLMCap = "llm"

// SystemPromptCap is the Capability Context Manager Plugins provide (ADR-0006).
const SystemPromptCap = "system-prompt"

// LoopCap is the replaceable Agent Loop Capability (ADR-0003). Host uses the
// in-process default unless a mounted Plugin provides this Capability.
const LoopCap = "loop"

// LLMChunkMethod is the evt method LLM Plugins use to stream a delta.
const LLMChunkMethod = "chunk"

// LLMCompleteMethod is the req/res method LLM Plugins implement.
const LLMCompleteMethod = "complete"

// ToolsCap is the Capability Tools Plugins provide (list/call).
const ToolsCap = "tools"

// SubagentToolName is the model-facing tool Host injects to spawn a Subagent (CONTEXT.md).
const SubagentToolName = "run_subagent"

// MaxSteps bounds model hops (Steps) inside one Turn (CONTEXT.md Turn/Step).
const MaxSteps = 128

// ToolSchema is one model-facing tool registration from tools.list.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// TurnResult is one default-Loop turn (ADR-0003: Loop compiled into Host).
type TurnResult struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Chunks    []string  `json:"chunks"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
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
// Empty sessionID targets the default Session.
func (s *Server) QuerySessionFacts(sessionID string, afterSeq, limit int) ([]map[string]any, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	payload := map[string]any{"afterSeq": afterSeq, "limit": limit}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "query",
		Payload: MarshalPayload(payload),
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
// Empty sessionID targets the default Session.
func (s *Server) DeriveMessages(sessionID string) ([]Message, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	payload := json.RawMessage(`{}`)
	if sessionID != "" {
		payload = MarshalPayload(map[string]string{"sessionId": sessionID})
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "derive",
		Payload: payload,
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
// Empty sessionID targets the default Session.
func (s *Server) AppendSessionFacts(sessionID string, facts []map[string]any) (int, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	last := 0
	for _, fact := range facts {
		body := fact
		if sessionID != "" {
			body = make(map[string]any, len(fact)+1)
			for k, v := range fact {
				body[k] = v
			}
			body["sessionId"] = sessionID
		}
		res, err := s.Call(owner, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     SessionCap,
			Method:  "append",
			Payload: MarshalPayload(body),
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
// Empty sessionID targets the default Session.
func (s *Server) AgentRequest(sessionID string, claimed []Message) (*AgentRequestResult, error) {
	derived, err := s.DeriveMessages(sessionID)
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

// AssembleSystemPrompt asks the mounted Context Manager for the assembled System Prompt.
// Returns empty text when no plugin provides system-prompt (ADR-0006).
func (s *Server) AssembleSystemPrompt() (string, error) {
	s.mu.Lock()
	owner, ok := s.provides[SystemPromptCap]
	s.mu.Unlock()
	if !ok {
		return "", nil
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SystemPromptCap,
		Method:  "assemble",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		return "", fmt.Errorf("system-prompt.assemble: %w", err)
	}
	if res.Error != nil {
		return "", fmt.Errorf("system-prompt.assemble: %w", res.Error)
	}
	var out struct {
		Text string `json:"text"`
	}
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return "", fmt.Errorf("system-prompt.assemble: bad payload: %w", err)
		}
	}
	return out.Text, nil
}

func (s *Server) emitStatus(status string) {
	if s.OnStatus != nil {
		s.OnStatus(status)
	}
	s.publish(Event{Topic: "status", Data: map[string]string{"status": status}})
}

// emitRenderIntent notifies the CLI hook and all Subscribers (Web Medium).
func (s *Server) emitRenderIntent(ri RenderIntent) {
	if s.OnRender != nil {
		s.OnRender(ri)
	}
	s.publish(Event{Topic: "presentation", Data: ri})
}

// extractStreamDelta returns text delta from llm.chunk or presentation.stream chunk frames.
func extractStreamDelta(f *protocol.Frame) (string, bool) {
	var c struct {
		Op    string `json:"op"`
		Delta string `json:"delta"`
	}
	if len(f.Payload) > 0 {
		_ = json.Unmarshal(f.Payload, &c)
	}
	if f.Method == LLMChunkMethod {
		return c.Delta, c.Delta != ""
	}
	if f.Cap == PresentationCap && f.Method == PresentationStreamMethod && c.Op == "chunk" {
		return c.Delta, c.Delta != ""
	}
	return "", false
}

// RunTurn is the Host-compiled default Agent Loop for one chat turn.
//
// If a mounted Plugin provides LoopCap, that external Loop is used (ADR-0003).
// Default flow: turn/start → system-prompt.assemble → session.append(system) → session.append(user)
// → per Step: step/start → AgentRequest(rebuild) → llm.complete(tools)
// → optional tools.call → session tool facts → step/end → repeat until final assistant reply
// → turn/end.
func (s *Server) RunTurn(userInput string) (*TurnResult, error) {
	return s.RunTurnOn("", userInput)
}

// RunTurnOn is RunTurn bound to a Session id (empty = default).
func (s *Server) RunTurnOn(sessionID, userInput string) (*TurnResult, error) {
	// One turn at a time across CLI and Web (spec §7 / ADR-0009).
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	s.turnCancel.Store(false)
	return s.runTurn(sessionID, userInput, true, "")
}

// CancelTurn requests the in-flight default Loop to stop at the next safe boundary.
func (s *Server) CancelTurn() {
	s.turnCancel.Store(true)
}

// TurnCancelled reports whether CancelTurn was requested.
func (s *Server) TurnCancelled() bool {
	return s.turnCancel.Load()
}

// NewSessionID creates a Session (auto id when empty) and returns the id.
func (s *Server) NewSessionID(id string) (string, error) {
	if id == "" {
		id = fmt.Sprintf("s-%d", time.Now().UnixNano())
	}
	if err := s.CreateSession(id, "", "web", 0); err != nil {
		return "", err
	}
	return id, nil
}

// runTurn executes one Turn on sessionID (empty = default).
// allowSubagent controls whether Host injects the run_subagent tool schema.
// extraSystem is an additional System Prompt fragment logged inside the Turn.
func (s *Server) runTurn(sessionID, userInput string, allowSubagent bool, extraSystem string) (*TurnResult, error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, fmt.Errorf("agent loop: user input is required")
	}

	s.mu.Lock()
	loopOwner, hasExternalLoop := s.provides[LoopCap]
	s.mu.Unlock()
	if hasExternalLoop {
		return s.runExternalTurn(loopOwner, sessionID, userInput)
	}

	if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
		"type": "turn_start",
		"role": "host",
		"meta": map[string]any{"turn": 1},
	}}); err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	s.emitStatus("running")
	turnFailed := true
	defer func() {
		reason := "completed"
		if turnFailed {
			reason = "error"
		}
		_, _ = s.AppendSessionFacts(sessionID, []map[string]any{{
			"type": "turn_end",
			"role": "host",
			"meta": map[string]any{"turn": 1, "reason": reason},
		}})
		s.emitStatus("idle")
	}()

	// System Prompt is assembled then logged before any model-visible user input (ADR-0005/0006).
	sysText, err := s.AssembleSystemPrompt()
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	if extraSystem != "" {
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += extraSystem
	}
	s.mu.Lock()
	_, hasToolsProvider := s.provides[ToolsCap]
	s.mu.Unlock()
	if !hasToolsProvider {
		note := "No tools are mounted in this assembly. Do not claim to use tools, browse the workspace, or run commands. Answer from the conversation only, or ask the user to use the agent assembly if they need file tools."
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += note
	}
	if sysText != "" {
		if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
			"type":    "message",
			"role":    "system",
			"content": sysText,
		}}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}
	}

	if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
		"type":    "message",
		"role":    "user",
		"content": userInput,
	}}); err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	schemas, err := s.CollectToolSchemas()
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	// Inject Subagent tool only when a Tools Plugin is mounted (avoids forcing
	// tool-calls on minimal session+llm assemblies).
	s.mu.Lock()
	_, hasTools := s.provides[ToolsCap]
	s.mu.Unlock()
	if allowSubagent && hasTools {
		schemas = append(schemas, ToolSchema{
			Name:        SubagentToolName,
			Description: "Run a focused subagent on a new session and return its final reply.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"},"text":{"type":"string"},"systemPrompt":{"type":"string"},"tools":{"type":"array","items":{"type":"string"}},"mode":{"type":"string","enum":["sync","async"]}},"required":["input"]}`),
		})
	}

	var allChunks []string
	var toolNames []string
	var assistant string

	for step := 0; step < MaxSteps; step++ {
		if s.turnCancel.Load() {
			_, _ = s.AppendSessionFacts(sessionID, []map[string]any{{
				"type": "step_end",
				"role": "host",
				"meta": map[string]any{"turn": 1, "step": max(step, 1), "reason": "cancelled"},
			}})
			return nil, fmt.Errorf("turn cancelled")
		}
		stepN := step + 1
		if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
			"type": "step_start",
			"role": "host",
			"meta": map[string]any{"turn": 1, "step": stepN},
		}}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		// Invariant: Model Context must be rebuildable from Session Log (ADR-0002).
		ar, err := s.AgentRequest(sessionID, nil)
		if err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		s.mu.Lock()
		llmOwner, ok := s.provides[LLMCap]
		s.mu.Unlock()
		if !ok {
			return nil, fmt.Errorf("agent loop: no plugin provides %q", LLMCap)
		}

		reqBody := map[string]any{"messages": ar.Messages}
		if len(schemas) > 0 {
			reqBody["tools"] = schemas
		}

		// Request Header snapshot for audit/replay (CONTEXT.md Request Header).
		if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
			"type": "request_header",
			"role": "host",
			"meta": map[string]any{
				"provider": "default",
				"model":    "default",
				"step":     stepN,
				"turn":     1,
			},
		}}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		out, err := s.CallStreamOn(llmOwner, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     LLMCap,
			Method:  LLMCompleteMethod,
			Payload: MarshalPayload(reqBody),
		}, func(ev *protocol.Frame) {
			if s.turnCancel.Load() {
				return
			}
			if delta, ok := extractStreamDelta(ev); ok {
				if s.OnStreamDelta != nil {
					s.OnStreamDelta(delta)
				}
				s.publish(Event{Topic: "stream", Data: map[string]string{"delta": delta}})
			}
		})
		if err != nil {
			return nil, fmt.Errorf("agent loop: llm.%s: %w", LLMCompleteMethod, err)
		}
		if out.Frame.Error != nil {
			return nil, fmt.Errorf("agent loop: llm.%s: %w", LLMCompleteMethod, out.Frame.Error)
		}

		var llmOut struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		}
		if len(out.Frame.Payload) > 0 {
			if err := json.Unmarshal(out.Frame.Payload, &llmOut); err != nil {
				return nil, fmt.Errorf("agent loop: llm.%s: bad payload: %w", LLMCompleteMethod, err)
			}
		}

		for _, ev := range out.Events {
			if delta, ok := extractStreamDelta(ev); ok {
				allChunks = append(allChunks, delta)
			}
		}

		// Final assistant reply (no tool calls).
		if len(llmOut.ToolCalls) == 0 {
			assistant = llmOut.Content
			if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
				"type":    "message",
				"role":    "assistant",
				"content": llmOut.Content,
			}}); err != nil {
				return nil, fmt.Errorf("agent loop: %w", err)
			}
			// Settle: durable assistant body as markdown_text for every Render Medium.
			if llmOut.Content != "" {
				s.emitRenderIntent(RenderIntent{Kind: KindMarkdownText, Text: llmOut.Content})
			}
			if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
				"type": "step_end",
				"role": "host",
				"meta": map[string]any{"turn": 1, "step": stepN, "reason": "completed"},
			}}); err != nil {
				return nil, fmt.Errorf("agent loop: %w", err)
			}
			break
		}

		// Last allowed model hop: do not execute tools we cannot feed back.
		if step == MaxSteps-1 {
			_, _ = s.AppendSessionFacts(sessionID, []map[string]any{{
				"type": "step_end",
				"role": "host",
				"meta": map[string]any{"turn": 1, "step": stepN, "reason": "max_steps"},
			}})
			return nil, fmt.Errorf("agent loop: exceeded %d steps", MaxSteps)
		}

		// Normalize tool-call ids: OpenAI/DeepSeek reject empty or duplicate tool_call_id.
		seenIDs := map[string]bool{}
		for i := range llmOut.ToolCalls {
			if llmOut.ToolCalls[i].ID == "" || seenIDs[llmOut.ToolCalls[i].ID] {
				llmOut.ToolCalls[i].ID = fmt.Sprintf("call_%d_%d", stepN, i+1)
			}
			seenIDs[llmOut.ToolCalls[i].ID] = true
		}

		// One assistant fact carries content (if any) + all tool_calls, then execute.
		callMeta := make([]map[string]any, 0, len(llmOut.ToolCalls))
		for _, tc := range llmOut.ToolCalls {
			toolNames = append(toolNames, tc.Name)
			item := map[string]any{
				"id":           tc.ID,
				"tool_call_id": tc.ID,
				"name":         tc.Name,
			}
			if len(tc.Arguments) > 0 {
				item["arguments"] = json.RawMessage(tc.Arguments)
			}
			callMeta = append(callMeta, item)
		}
		if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
			"type":    "tool_call",
			"role":    "assistant",
			"content": llmOut.Content,
			"meta":    map[string]any{"tool_calls": callMeta},
		}}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		for _, tc := range llmOut.ToolCalls {
			toolOut, callErr := s.CallTool(tc)
			resultContent := ""
			var additionalContexts []Message
			if callErr != nil {
				resultContent = "error: " + callErr.Error()
			} else {
				resultContent = toolOut.Content
				additionalContexts = toolOut.AdditionalContexts
			}
			// Render Medium: summary_text card (pairs from args, detail = result + overflow).
			pairs, overflow := JSONToPairs(tc.Arguments, MaxSummaryPairs)
			detail := resultContent
			if overflow != "" {
				if detail != "" {
					detail += "\n"
				}
				detail += overflow
			}
			s.emitRenderIntent(RenderIntent{
				Kind:   KindSummaryText,
				Title:  tc.Name,
				Pairs:  pairs,
				Detail: TruncateRunes(detail, MaxSummaryDetail),
			})
			if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
				"type":    "tool_result",
				"role":    "tool",
				"content": resultContent,
				"meta":    map[string]any{"tool_call_id": tc.ID},
			}}); err != nil {
				return nil, fmt.Errorf("agent loop: %w", err)
			}
			// Additional Contexts land after the tool result (CONTEXT / US16).
			for _, ac := range additionalContexts {
				if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
					"type":    "message",
					"role":    defaultSystemRole(ac.Role),
					"content": ac.Content,
				}}); err != nil {
					return nil, fmt.Errorf("agent loop: %w", err)
				}
			}
		}

		if _, err := s.AppendSessionFacts(sessionID, []map[string]any{{
			"type": "step_end",
			"role": "host",
			"meta": map[string]any{"turn": 1, "step": stepN, "reason": "tools"},
		}}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}
	}

	msgs, err := s.DeriveMessages(sessionID)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	turnFailed = false
	return &TurnResult{
		User:      userInput,
		Assistant: assistant,
		Chunks:    allChunks,
		ToolCalls: toolNames,
		Messages:  msgs,
	}, nil
}

// CollectToolSchemas asks the mounted Tools Plugin for model-facing tool registrations.
func (s *Server) CollectToolSchemas() ([]ToolSchema, error) {
	s.mu.Lock()
	owner, ok := s.provides[ToolsCap]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     ToolsCap,
		Method:  "list",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		return nil, fmt.Errorf("tools.list: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("tools.list: %w", res.Error)
	}
	var out struct {
		Tools []ToolSchema `json:"tools"`
	}
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return nil, fmt.Errorf("tools.list: bad payload: %w", err)
		}
	}
	return out.Tools, nil
}

// CallToolResult is one tools.call outcome, including Additional Contexts (CONTEXT.md).
type CallToolResult struct {
	Content            string    `json:"content"`
	AdditionalContexts []Message `json:"additionalContexts,omitempty"`
}

// CreateSession creates a Session by id via the mounted Session Plugin (idempotent).
func (s *Server) CreateSession(sessionID, parentSession, origin string, delegationDepth int) error {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no plugin provides %q", SessionCap)
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:      protocol.Version,
		Type:   protocol.TypeReq,
		Cap:    SessionCap,
		Method: "create",
		Payload: MarshalPayload(map[string]any{
			"sessionId":       sessionID,
			"parentSession":   parentSession,
			"origin":          origin,
			"delegationDepth": delegationDepth,
		}),
	})
	if err != nil {
		return fmt.Errorf("session.create: %w", err)
	}
	if res.Error != nil {
		return fmt.Errorf("session.create: %w", res.Error)
	}
	return nil
}

// RunSubagent spawns a Subagent: new Session + default Loop (allowSubagent=false to avoid recursion).
// v1 supports mode=sync only; async returns an error (spec Out of Scope / ticket 06).
func (s *Server) RunSubagent(input, systemPrompt, mode string, toolFilter []string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", &protocol.FrameError{Code: "bad_payload", Message: "subagent input is required"}
	}
	if mode == "async" {
		return "", &protocol.FrameError{Code: "not_supported", Message: "async subagent is not supported in v1"}
	}
	if mode == "" {
		mode = "sync"
	}
	s.mu.Lock()
	s.seq++
	childID := fmt.Sprintf("subagent-%d", s.seq)
	s.mu.Unlock()

	// Parent is the default Session when spawned from the default Turn (v1).
	if err := s.CreateSession(childID, "default", "subagent", 1); err != nil {
		return "", err
	}
	// v1: toolFilter reserved; child does not inherit run_subagent (no recursion).
	_ = toolFilter

	// extraSystem is appended inside the child Turn after turn_start (not outside the boundary).
	res, err := s.runTurn(childID, input, false, systemPrompt)
	if err != nil {
		return "", err
	}
	return res.Assistant, nil
}

// CallTool executes one ToolCall through the mounted Tools Plugin,
// or Host-run Subagent when the tool is run_subagent.
func (s *Server) CallTool(tc ToolCall) (*CallToolResult, error) {
	if s.OnToolCall != nil {
		s.OnToolCall(tc.Name, tc.Arguments)
	}
	s.emitRenderIntent(RenderIntent{Kind: KindMessageText, Level: "info", Text: formatRunningLine(tc.Name)})
	if tc.Name == SubagentToolName {
		var in struct {
			Input        string   `json:"input"`
			Text         string   `json:"text"`
			SystemPrompt string   `json:"systemPrompt"`
			Mode         string   `json:"mode"`
			Tools        []string `json:"tools"`
		}
		if len(tc.Arguments) > 0 {
			if err := json.Unmarshal(tc.Arguments, &in); err != nil {
				return nil, fmt.Errorf("tools.call %s: bad arguments: %w", SubagentToolName, err)
			}
		}
		input := in.Input
		if input == "" {
			input = in.Text
		}
		out, err := s.RunSubagent(input, in.SystemPrompt, in.Mode, in.Tools)
		if err != nil {
			return nil, err
		}
		return &CallToolResult{Content: out}, nil
	}

	s.mu.Lock()
	owner, ok := s.provides[ToolsCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", ToolsCap)
	}
	args := tc.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:      protocol.Version,
		Type:   protocol.TypeReq,
		Cap:    ToolsCap,
		Method: "call",
		Payload: MarshalPayload(map[string]any{
			"name":      tc.Name,
			"arguments": json.RawMessage(args),
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("tools.call %s: %w", tc.Name, err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("tools.call %s: %w", tc.Name, res.Error)
	}
	var out CallToolResult
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return nil, fmt.Errorf("tools.call %s: bad payload: %w", tc.Name, err)
		}
	}
	return &out, nil
}

func (s *Server) runExternalTurn(owner, sessionID, userInput string) (*TurnResult, error) {
	payload := map[string]string{"input": userInput}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     LoopCap,
		Method:  "turn",
		Payload: MarshalPayload(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("agent loop: %w", res.Error)
	}
	var out TurnResult
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return nil, fmt.Errorf("agent loop: bad loop.turn payload: %w", err)
		}
	}
	if out.User == "" {
		out.User = userInput
	}
	return &out, nil
}

func messagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
		if a[i].ToolCallID != b[i].ToolCallID {
			return false
		}
		if len(a[i].ToolCalls) != len(b[i].ToolCalls) {
			return false
		}
		for j := range a[i].ToolCalls {
			x, y := a[i].ToolCalls[j], b[i].ToolCalls[j]
			if x.ID != y.ID || x.Name != y.Name || !bytesEqualJSON(x.Arguments, y.Arguments) {
				return false
			}
		}
	}
	return true
}

func bytesEqualJSON(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return string(a) == string(b)
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
	default:
		res.Error = &protocol.FrameError{
			Code:    "method_not_found",
			Message: fmt.Sprintf("no handler for agent.%s", f.Method),
		}
		_ = s.writeTo(from, res)
	}
}

// AgentInject appends model-visible messages to the Session Log without starting a turn (US17).
// Accepts {"role","content"} or {"messages":[{role,content},...]} or optional sessionId.
func (s *Server) AgentInject(payload json.RawMessage) (map[string]int, error) {
	var in struct {
		SessionID string    `json:"sessionId"`
		Role      string    `json:"role"`
		Content   string    `json:"content"`
		Messages  []Message `json:"messages"`
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
	}
	msgs := in.Messages
	if len(msgs) == 0 && (in.Role != "" || in.Content != "") {
		msgs = []Message{{Role: in.Role, Content: in.Content}}
	}
	if len(msgs) == 0 {
		return nil, &protocol.FrameError{Code: "bad_payload", Message: "inject requires role/content or messages"}
	}
	facts := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		facts = append(facts, map[string]any{
			"type":    "message",
			"role":    defaultSystemRole(m.Role),
			"content": m.Content,
		})
	}
	seq, err := s.AppendSessionFacts(in.SessionID, facts)
	if err != nil {
		return nil, err
	}
	return map[string]int{"count": len(facts), "lastSeq": seq}, nil
}

func defaultSystemRole(role string) string {
	if role == "" {
		return "system"
	}
	return role
}
