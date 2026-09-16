// Command agent is the default Agent Plugin (ADR-0016): provides loop and
// composes llm / session / tools / system-prompt / context through the star.
//
// Host owns locks, Cancel/running status, and agent.request / agent.inject.
// This plugin owns Agent Loop policy: Turn/Step boundary facts, MaxSteps,
// run_subagent (child turn re-enters loop.turn on a child Session).
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// MaxSteps bounds model hops (Steps) inside one Turn.
const MaxSteps = 128

// MaxSubagentsPerTurn caps run_subagent spawns inside one parent Turn.
// Subagent turns re-run the full Loop (system/tools/prepare/llm) on a child
// Session — each spawn is roughly another complete conversation, so the
// default is deliberately tight.
const MaxSubagentsPerTurn = 1

const subagentToolName = "run_subagent"
const todoToolName = "todo"

// subagentToolDescription steers the model away from last-resort spawns.
// Keep this conservative: vague copy is what made the tool look free.
const subagentToolDescription = "LAST RESORT: spawn one focused subagent on a child session (full isolated Loop) and return its final reply. Prefer answering directly or using existing tools (read/edit/search). Use only for a self-contained subtask that benefits from a clean context — e.g. broad multi-file survey that would pollute the parent chat. Do NOT use for simple questions, single file reads, or tasks you can finish in this turn. Budget: at most once per turn."

var subagentSchema = map[string]any{
	"name":        subagentToolName,
	"description": subagentToolDescription,
	"input_schema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":        map[string]any{"type": "string", "description": "Self-contained subtask prompt for the child agent. Must not require parent chat context."},
			"text":         map[string]any{"type": "string"},
			"systemPrompt": map[string]any{"type": "string"},
			"tools":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"mode":         map[string]any{"type": "string", "enum": []string{"sync", "async"}},
		},
		"required": []string{"input"},
	},
}

var todoSchema = map[string]any{
	"name":        todoToolName,
	"description": "Update or list the visible work plan. Provide items with status pending|in_progress|done, or action=list.",
	"input_schema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"update", "list"}, "description": "default update"},
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status":  map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "done"}},
						"content": map[string]any{"type": "string"},
					},
					"required": []string{"status", "content"},
				},
			},
		},
	},
}

// subagentSystemNote is appended to the system prompt when the tool is offered.
const subagentSystemNote = "Subagent policy: run_subagent is a last-resort tool (max once per turn). Prefer direct answers and ordinary tools. Never spawn a subagent for greetings, simple Q&A, or a single tool call. Only spawn when the subtask is independent and would otherwise bloat this conversation. Subagents cannot spawn further subagents."

func subagentToolSchema() toolSchema {
	return toolSchema{
		Name:        subagentToolName,
		Description: subagentToolDescription,
		InputSchema: marshal(subagentSchema["input_schema"]),
	}
}

func todoToolSchema() toolSchema {
	return toolSchema{
		Name:        todoToolName,
		Description: "Update the visible work plan. Provide items with status pending|in_progress|done.",
		InputSchema: marshal(todoSchema["input_schema"]),
	}
}

func hasExternalTools(schemas []toolSchema) bool {
	for _, s := range schemas {
		if s.Name != subagentToolName && s.Name != todoToolName && s.Name != "" {
			return true
		}
	}
	return false
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type toolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	// ReadOnly marks non-mutating tools for parallel scheduling (spec).
	ReadOnly bool `json:"readOnly,omitempty"`
}

type turnResult struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Chunks    []string  `json:"chunks,omitempty"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
	Messages  []message `json:"messages,omitempty"`
}

// cancelState is Host→agent cancel for an in-flight turn (ADR-0016).
type cancelState struct {
	mu sync.Mutex
	m  map[string]bool
}

func newCancelState() *cancelState {
	return &cancelState{m: make(map[string]bool)}
}

func (c *cancelState) request(sessionID string) {
	c.mu.Lock()
	c.m[normalizeID(sessionID)] = true
	c.mu.Unlock()
}

func (c *cancelState) take(sessionID string) bool {
	id := normalizeID(sessionID)
	c.mu.Lock()
	v := c.m[id]
	c.mu.Unlock()
	return v
}

func (c *cancelState) clear(sessionID string) {
	c.mu.Lock()
	delete(c.m, normalizeID(sessionID))
	c.mu.Unlock()
}

func normalizeID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "default"
	}
	return id
}

func marshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func callJSON(s *pluginsdk.Server, cap, method string, payload any) (json.RawMessage, error) {
	raw := marshal(payload)
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	out, err := s.Call(cap, method, raw)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (a *agent) appendOne(sessionID string, fact map[string]any) error {
	if sessionID != "" {
		cp := make(map[string]any, len(fact)+1)
		for k, v := range fact {
			cp[k] = v
		}
		cp["sessionId"] = sessionID
		fact = cp
	}
	_, err := callJSON(a.s, "session", "append", fact)
	return err
}

func (a *agent) deriveMessages(sessionID string) ([]message, error) {
	payload := map[string]any{}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	raw, err := callJSON(a.s, "session", "derive", payload)
	if err != nil {
		return nil, err
	}
	var out struct {
		Messages []message `json:"messages"`
	}
	// Some builds return the array directly.
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &out.Messages)
		return out.Messages, nil
	}
	_ = json.Unmarshal(raw, &out)
	return out.Messages, nil
}

func (a *agent) agentRequest(sessionID string) ([]message, error) {
	payload := map[string]any{}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	raw, err := callJSON(a.s, "agent", "request", payload)
	if err != nil {
		// Fall back to session.derive when agent.request is unavailable.
		return a.deriveMessages(sessionID)
	}
	var out struct {
		Messages []message `json:"messages"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Messages, nil
}

func (a *agent) assembleSystemPrompt() string {
	raw, err := callJSON(a.s, "system-prompt", "assemble", map[string]any{})
	if err != nil || len(raw) == 0 {
		return ""
	}
	var out struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Text != "" {
		return out.Text
	}
	return out.Content
}

func (a *agent) collectToolSchemas() ([]toolSchema, bool) {
	raw, err := callJSON(a.s, "tools", "list", map[string]any{})
	if err != nil {
		return nil, false
	}
	var out struct {
		Tools []toolSchema `json:"tools"`
	}
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &out.Tools)
	} else {
		_ = json.Unmarshal(raw, &out)
	}
	// toolsOK means a tools Provider is mounted (even if it registered zero tools).
	return out.Tools, true
}

func (a *agent) llmInfoContextWindow() int {
	raw, err := callJSON(a.s, "llm", "info", map[string]any{})
	if err != nil || len(raw) == 0 {
		return 0
	}
	var out struct {
		ContextWindow int `json:"contextWindow"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.ContextWindow
}

func (a *agent) prepareContext(sessionID string, msgs []message, contextWindow int) ([]message, []toolSchema, map[string]any, error) {
	payload := map[string]any{"messages": msgs}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	if contextWindow > 0 {
		payload["contextWindow"] = contextWindow
	}
	raw, err := callJSON(a.s, "context", "prepare", payload)
	if err != nil {
		return nil, nil, nil, err
	}
	var out struct {
		Messages    []message      `json:"messages"`
		Tools       []toolSchema   `json:"tools"`
		CompactHint map[string]any `json:"compactHint"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Messages, out.Tools, out.CompactHint, nil
}

// maybeAutoCompact appends a Context Summary when prepare suggests compact (Phase 2).
func (a *agent) maybeAutoCompact(sessionID string, msgs []message, hint map[string]any) {
	if hint == nil {
		return
	}
	suggest, _ := hint["suggestCompact"].(bool)
	if !suggest {
		return
	}
	covers := 0
	if raw, err := callJSON(a.s, "session", "query", map[string]any{
		"sessionId": sessionID, "afterSeq": 0, "limit": 0,
	}); err == nil {
		var q struct {
			Facts []struct {
				Seq int `json:"seq"`
			} `json:"facts"`
		}
		_ = json.Unmarshal(raw, &q)
		if n := len(q.Facts); n > 0 {
			// Cover everything currently projected (pre-summary history).
			covers = q.Facts[n-1].Seq
		}
	}
	raw, err := callJSON(a.s, "context", "compact", map[string]any{
		"sessionId":        sessionID,
		"messages":         msgs,
		"coversThroughSeq": covers,
	})
	if err != nil {
		return
	}
	var out struct {
		Summary          string `json:"summary"`
		CoversThroughSeq int    `json:"coversThroughSeq"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Summary == "" {
		return
	}
	_ = a.appendOne(sessionID, map[string]any{
		"type":    "context_summary",
		"role":    "system",
		"content": out.Summary,
		"meta": map[string]any{
			"active":           true,
			"coversThroughSeq": out.CoversThroughSeq,
			"auto":             true,
		},
	})
}

func (a *agent) nextTurnNumber(sessionID string) int {
	payload := map[string]any{"afterSeq": 0, "limit": 0}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	raw, err := callJSON(a.s, "session", "query", payload)
	if err != nil {
		return 1
	}
	var out struct {
		Facts []struct {
			Type string `json:"type"`
			Meta struct {
				Turn int `json:"turn"`
			} `json:"meta"`
		} `json:"facts"`
	}
	_ = json.Unmarshal(raw, &out)
	maxTurn := 0
	for _, f := range out.Facts {
		if f.Type == "turn_start" && f.Meta.Turn > maxTurn {
			maxTurn = f.Meta.Turn
		}
	}
	return maxTurn + 1
}

func (a *agent) sessionWorkspace(sessionID string) string {
	raw, err := callJSON(a.s, "session", "info", map[string]any{"sessionId": sessionID})
	if err != nil {
		return ""
	}
	var out struct {
		Workspace string `json:"workspace"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Workspace
}

// policyDecide asks the policy Capability (ADR-0019). Missing provider → allow.
// severity is the tool author's declared risk (low|medium|high) from tools.list.
func (a *agent) policyDecide(sessionID, tool string, args json.RawMessage, workspace, severity string) (string, string) {
	payload := map[string]any{
		"tool":      tool,
		"arguments": args,
		"workspace": workspace,
		"sessionId": sessionID,
	}
	if severity != "" {
		payload["severity"] = severity
	}
	raw, err := callJSON(a.s, "policy", "decide", payload)
	if err != nil {
		return "allow", "no policy provider"
	}
	var out struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Action == "" {
		return "allow", "empty policy action"
	}
	return out.Action, out.Reason
}

func (a *agent) confirmTool(sessionID, tool string, args json.RawMessage, workspace string) bool {
	raw, err := callJSON(a.s, "agent", "confirm", map[string]any{
		"tool":      tool,
		"arguments": args,
		"workspace": workspace,
		"sessionId": sessionID,
	})
	if err != nil {
		return false
	}
	var out struct {
		Approved bool `json:"approved"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Approved
}

func (a *agent) expandSkillTriggers(sessionID, workspace, input string) string {
	raw, err := callJSON(a.s, "skills", "expand", map[string]any{
		"workspace": workspace,
		"text":      input,
	})
	if err != nil {
		return input
	}
	var out struct {
		Text     string   `json:"text"`
		Injected []string `json:"injected"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Text == "" {
		return input
	}
	return out.Text
}

func (a *agent) loadProjectContext(workspace, extra string) string {
	if workspace == "" {
		return extra
	}
	raw, err := callJSON(a.s, "project-context", "load", map[string]any{"workspace": workspace})
	if err != nil {
		return extra
	}
	var out struct {
		Text   string `json:"text"`
		Source string `json:"source"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Text == "" {
		return extra
	}
	block := "Project context (" + out.Source + "):\n" + out.Text
	// Prefer CM Prompt Segment (assemble includes it). Fallback: extraSystem.
	seg, _ := json.Marshal(map[string]any{
		"name":  "project-context",
		"order": 30,
		"text":  block,
	})
	if _, err := callJSON(a.s, "system-prompt", "registerSegment", json.RawMessage(seg)); err != nil {
		if extra == "" {
			return block
		}
		return extra + "\n\n" + block
	}
	return extra
}

func (a *agent) refreshSkillCatalog(workspace string) {
	_, _ = callJSON(a.s, "skills", "refreshCatalog", map[string]any{"workspace": workspace})
}

func (a *agent) handleTodo(sessionID string, tc toolCall) (string, error) {
	var in struct {
		Action string `json:"action"`
		Items  []struct {
			Status  string `json:"status"`
			Content string `json:"content"`
		} `json:"items"`
	}
	if len(tc.Arguments) > 0 {
		if err := json.Unmarshal(tc.Arguments, &in); err != nil {
			return "", err
		}
	}
	if in.Action == "list" || (in.Action == "" && len(in.Items) == 0) {
		raw, err := callJSON(a.s, "session", "query", map[string]any{"sessionId": sessionID, "afterSeq": 0, "limit": 0})
		if err != nil {
			return "", err
		}
		var q struct {
			Facts []struct {
				Type string `json:"type"`
				Body string `json:"content"`
			} `json:"facts"`
		}
		_ = json.Unmarshal(raw, &q)
		last := "(no todo yet — call with items to create one)"
		for _, f := range q.Facts {
			if f.Type == "todo" {
				last = f.Body
			}
		}
		return last, nil
	}
	var b strings.Builder
	b.WriteString("Todo updated:\n")
	for _, it := range in.Items {
		mark := " "
		switch it.Status {
		case "done":
			mark = "x"
		case "in_progress":
			mark = "-"
		}
		fmt.Fprintf(&b, "- [%s] (%s) %s\n", mark, it.Status, it.Content)
	}
	body := strings.TrimSpace(b.String())
	if err := a.appendOne(sessionID, map[string]any{
		"type":    "todo",
		"role":    "tool",
		"content": body,
		"meta":    map[string]any{"tool_call_id": tc.ID},
	}); err != nil {
		return "", err
	}
	return body, nil
}

func (a *agent) logPolicyDecision(sessionID, tool, action, reason string) {
	_ = a.appendOne(sessionID, map[string]any{
		"type":    "policy_decision",
		"role":    "host",
		"content": tool + " → " + action,
		"meta":    map[string]any{"tool": tool, "action": action, "reason": reason},
	})
}

func (a *agent) callTool(sessionID string, tc toolCall, severity string) (string, []message, error) {
	args := tc.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	workspace := a.sessionWorkspace(sessionID)
	// Policy plugin owns the full verdict (including ask → Medium round-trip).
	action, reason := a.policyDecide(sessionID, tc.Name, args, workspace, severity)
	if action == "deny" {
		a.logPolicyDecision(sessionID, tc.Name, "deny", reason)
		return "error: denied by policy: " + reason, nil, nil
	}
	if action == "ask" {
		// Fallback only: a policy provider that returns ask without resolving
		// the Medium itself. Default sandbox resolves ask inside decide.
		ok := a.confirmTool(sessionID, tc.Name, args, workspace)
		if !ok {
			a.logPolicyDecision(sessionID, tc.Name, "ask-denied", reason)
			return "error: denied by user approval: " + reason, nil, nil
		}
		a.logPolicyDecision(sessionID, tc.Name, "ask-allowed", reason)
	}
	payload := map[string]any{
		"name":      tc.Name,
		"arguments": json.RawMessage(args),
		"workspace": workspace,
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	raw, err := callJSON(a.s, "tools", "call", payload)
	if err != nil {
		return "", nil, err
	}
	var out struct {
		Content            string    `json:"content"`
		AdditionalContexts []message `json:"additionalContexts"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Content, out.AdditionalContexts, nil
}

func isReadOnlySchema(s map[string]any) bool {
	v, ok := s["readOnly"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func messagesEqual(a, b []message) bool {
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
			if x.ID != y.ID || x.Name != y.Name || string(x.Arguments) != string(y.Arguments) {
				return false
			}
		}
	}
	return true
}

type agent struct {
	s            *pluginsdk.Server
	cancel       *cancelState
	contextProbe bool
	seq          int
	seqMu        sync.Mutex
}

func (a *agent) nextSubagentID() string {
	a.seqMu.Lock()
	a.seq++
	n := a.seq
	a.seqMu.Unlock()
	return fmt.Sprintf("subagent-%d-%d", time.Now().UnixNano(), n)
}

func main() {
	a := &agent{
		s:      pluginsdk.New(),
		cancel: newCancelState(),
	}
	// Discover whether a context provider exists (soft probe at first turn).
	a.contextProbe = false

	a.s.Handle("loop", "turn", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Input         string `json:"input"`
			SessionID     string `json:"sessionId"`
			AllowSubagent *bool  `json:"allowSubagent"`
			ExtraSystem   string `json:"extraSystem"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		allowSub := true
		if in.AllowSubagent != nil {
			allowSub = *in.AllowSubagent
		}
		res, err := a.runTurn(in.SessionID, in.Input, allowSub, in.ExtraSystem)
		if err != nil {
			return nil, err
		}
		return marshal(res), nil
	})

	// Host CancelTurnOn fans out a loop.cancel evt to this plugin.
	a.s.Handle("loop", "cancel", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		a.cancel.request(in.SessionID)
		return json.RawMessage(`{"ok":true}`), nil
	})

	a.s.Handle("config", "get", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("get", req.Payload)
	})
	a.s.Handle("config", "set", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("set", req.Payload)
	})
	a.s.Handle("config", "schema", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("schema", req.Payload)
	})
	a.s.Handle("config", "reload", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("reload", req.Payload)
	})
	a.s.Handle("commands", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Command string `json:"command"`
			Args    string `json:"args"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		if in.Command != "config" {
			return nil, &protocol.FrameError{Code: "unknown_command", Message: "/agent " + in.Command}
		}
		return handleConfigCommand(in.Args)
	})

	_ = a.s.Serve()
}

func (a *agent) runTurn(sessionID, userInput string, allowSubagent bool, extraSystem string) (*turnResult, error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, &protocol.FrameError{Code: "bad_payload", Message: "user input is required"}
	}
	sessionID = normalizeID(sessionID)
	// Resolve Workspace once per Turn (ADR-0020); tools.call injects this value.
	workspace := a.sessionWorkspace(sessionID)
	// Skill Trigger ($name) expands before the fact is written (spec).
	userInput = a.expandSkillTriggers(sessionID, workspace, userInput)
	// Entry turns clear any stale cancel from a previous turn on this Session.
	// Nested subagent turns share the parent's cancel only via Host.
	a.refreshSkillCatalog(workspace)

	schemeName, sc := a.activeScheme()
	if err := a.ensureSchemePlugins(sc.DependsPlugins); err != nil {
		return nil, &protocol.FrameError{
			Code:    "scheme_ensure_failed",
			Message: fmt.Sprintf("scheme %s: %v", schemeName, err),
		}
	}
	maxSteps := schemeMaxSteps(sc)
	offerSubagentPolicy := allowSubagent && schemeBool(sc.RunSubagent, true)
	offerTodoPolicy := schemeBool(sc.Todo, true)

	turnN := a.nextTurnNumber(sessionID)
	if err := a.appendOne(sessionID, map[string]any{
		"type": "turn_start",
		"role": "host",
		"meta": map[string]any{"turn": turnN, "scheme": schemeName},
	}); err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	turnFailed := true
	defer func() {
		meta := map[string]any{"turn": turnN, "reason": "completed"}
		if turnFailed {
			meta["reason"] = "error"
		}
		_ = a.appendOne(sessionID, map[string]any{
			"type": "turn_end",
			"role": "host",
			"meta": meta,
		})
		a.cancel.clear(sessionID)
	}()

	sysText := a.assembleSystemPrompt()
	extraSystem = a.loadProjectContext(workspace, extraSystem)
	if extraSystem != "" {
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += extraSystem
	}

	schemas, toolsOK := a.collectToolSchemas()
	schemas = filterTools(schemas, sc)
	hasTools := toolsOK
	if !hasTools {
		note := "No tools are mounted in this assembly. Do not claim to use tools, browse the workspace, or run commands. Answer from the conversation only, or ask the user to use the agent assembly if they need file tools."
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += note
	} else if hasExternalTools(schemas) {
		// Plan Constraint (lite): prompt-only; no tool-mode gate (spec Q17).
		planNote := "Plan Constraint: Prefer to outline a short plan and track items with the todo tool before editing files or running commands. There is no separate plan mode; tools remain available."
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += planNote
	}

	// Soft policy when the tool is offered this turn: models overuse
	// under-specified "spawn helper" tools unless told when not to.
	offerSubagent := offerSubagentPolicy && hasTools
	if offerSubagent {
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += subagentSystemNote
	}

	if sysText != "" {
		// Skip re-append when identical system is already the active one after summary.
		skip := false
		if raw, err := callJSON(a.s, "session", "query", map[string]any{
			"sessionId": sessionID, "afterSeq": 0, "limit": 0,
		}); err == nil {
			var q struct {
				Facts []struct {
					Type    string `json:"type"`
					Role    string `json:"role"`
					Content string `json:"content"`
					Meta    struct {
						Active bool `json:"active"`
					} `json:"meta"`
				} `json:"facts"`
			}
			_ = json.Unmarshal(raw, &q)
			sawSummary := false
			lastSys := ""
			for _, f := range q.Facts {
				if f.Type == "context_summary" && f.Meta.Active {
					sawSummary = true
					lastSys = ""
					continue
				}
				if f.Type == "message" && f.Role == "system" {
					lastSys = f.Content
				}
			}
			if lastSys == sysText && (!sawSummary || lastSys != "") {
				// Match Host behavior: compare against system after active summary.
				if lastSys == sysText {
					skip = true
				}
			}
		}
		if !skip {
			if err := a.appendOne(sessionID, map[string]any{
				"type": "message", "role": "system", "content": sysText,
			}); err != nil {
				return nil, fmt.Errorf("agent loop: %w", err)
			}
		}
	}

	if err := a.appendOne(sessionID, map[string]any{
		"type": "message", "role": "user", "content": userInput,
	}); err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}

	if offerSubagent {
		schemas = append(schemas, subagentToolSchema())
	}
	// Todo only when real external tools exist and the scheme allows it.
	if offerTodoPolicy && hasExternalTools(schemas) {
		schemas = append(schemas, todoToolSchema())
	}

	contextWindow := a.llmInfoContextWindow()
	var allChunks []string
	var toolNames []string
	var assistant string
	subagentUsed := 0
	autoCompacted := false

	for step := 0; step < maxSteps; step++ {
		if a.cancel.take(sessionID) {
			_ = a.appendOne(sessionID, map[string]any{
				"type": "step_end",
				"role": "host",
				"meta": map[string]any{"turn": turnN, "step": max(step, 1), "reason": "cancelled"},
			})
			return nil, &protocol.FrameError{Code: "cancelled", Message: "turn cancelled"}
		}
		stepN := step + 1
		if err := a.appendOne(sessionID, map[string]any{
			"type": "step_start",
			"role": "host",
			"meta": map[string]any{"turn": turnN, "step": stepN},
		}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		arMsgs, err := a.agentRequest(sessionID)
		if err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}
		modelMessages := arMsgs

		if a.contextProbe {
			prepMsgs, prepTools, hint, perr := a.prepareContext(sessionID, arMsgs, contextWindow)
			if perr == nil {
				compacted := false
				if !autoCompacted && hint != nil {
					if suggest, _ := hint["suggestCompact"].(bool); suggest {
						a.maybeAutoCompact(sessionID, arMsgs, hint)
						compacted = true
						autoCompacted = true
					}
				}
				if compacted {
					if ar2, err := a.agentRequest(sessionID); err == nil && len(ar2) > 0 {
						arMsgs = ar2
						modelMessages = ar2
					}
				} else if len(prepMsgs) > 0 && !messagesEqual(prepMsgs, arMsgs) {
					return nil, &protocol.FrameError{
						Code:    "session_invariant_violation",
						Message: "context.prepare messages are not reconstructable from session log",
					}
				}
				if len(prepTools) > 0 {
					schemas = filterTools(prepTools, sc)
					// Drop the tool once the turn budget is spent so the model
					// stops seeing it as an available next step.
					if offerSubagent && subagentUsed < MaxSubagentsPerTurn {
						schemas = append(schemas, subagentToolSchema())
					}
					if offerTodoPolicy && hasExternalTools(schemas) {
						schemas = append(schemas, todoToolSchema())
					}
				}
			}
		} else {
			// First successful prepare marks the provider present.
			if _, _, _, perr := a.prepareContext(sessionID, arMsgs, contextWindow); perr == nil {
				a.contextProbe = true
			}
		}

		reqBody := map[string]any{"messages": modelMessages, "sessionId": sessionID}
		if len(schemas) > 0 {
			reqBody["tools"] = schemas
		}

		if err := a.appendOne(sessionID, map[string]any{
			"type": "request_header",
			"role": "host",
			"meta": map[string]any{
				"provider": "default",
				"model":    "default",
				"step":     stepN,
				"turn":     turnN,
			},
		}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		llmRaw, err := callJSON(a.s, "llm", "complete", reqBody)
		if err != nil {
			return nil, fmt.Errorf("agent loop: llm.complete: %w", err)
		}
		var llmOut struct {
			Content   string         `json:"content"`
			ToolCalls []toolCall     `json:"tool_calls"`
			Usage     map[string]any `json:"usage"`
		}
		if len(llmRaw) > 0 {
			if err := json.Unmarshal(llmRaw, &llmOut); err != nil {
				return nil, fmt.Errorf("agent loop: llm.complete: bad payload: %w", err)
			}
		}

		if len(llmOut.Usage) > 0 {
			_ = a.appendOne(sessionID, map[string]any{
				"type": "llm_usage",
				"role": "host",
				"meta": map[string]any{
					"step":  stepN,
					"turn":  turnN,
					"usage": llmOut.Usage,
				},
			})
			// Best-effort: surface to Context Manager.
			_, _ = callJSON(a.s, "context", "noteUsage", map[string]any{
				"sessionId": sessionID, "usage": llmOut.Usage,
			})
		}

		if len(llmOut.ToolCalls) == 0 {
			assistant = llmOut.Content
			if err := a.appendOne(sessionID, map[string]any{
				"type": "message", "role": "assistant", "content": llmOut.Content,
			}); err != nil {
				return nil, fmt.Errorf("agent loop: %w", err)
			}
			if llmOut.Content != "" {
				_ = a.s.Emit("presentation", "render", marshal(map[string]any{
					"kind": "markdown_text", "text": llmOut.Content, "sessionId": sessionID,
				}))
			}
			_ = a.appendOne(sessionID, map[string]any{
				"type": "step_end",
				"role": "host",
				"meta": map[string]any{"turn": turnN, "step": stepN, "reason": "completed"},
			})
			break
		}

		if step == MaxSteps-1 {
			_ = a.appendOne(sessionID, map[string]any{
				"type": "step_end",
				"role": "host",
				"meta": map[string]any{"turn": turnN, "step": stepN, "reason": "max_steps"},
			})
			return nil, fmt.Errorf("agent loop: exceeded %d steps", MaxSteps)
		}

		seenIDs := map[string]bool{}
		for i := range llmOut.ToolCalls {
			if llmOut.ToolCalls[i].ID == "" || seenIDs[llmOut.ToolCalls[i].ID] {
				llmOut.ToolCalls[i].ID = fmt.Sprintf("call_%d_%d", stepN, i+1)
			}
			seenIDs[llmOut.ToolCalls[i].ID] = true
		}

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
		if err := a.appendOne(sessionID, map[string]any{
			"type":    "tool_call",
			"role":    "assistant",
			"content": llmOut.Content,
			"meta":    map[string]any{"tool_calls": callMeta},
		}); err != nil {
			return nil, fmt.Errorf("agent loop: %w", err)
		}

		// Partition: read-only tools may run in parallel; writes stay serial.
		// Capture author-declared severity from tools.list for policy.decide.
		readOnlyTools := map[string]bool{}
		toolSeverity := map[string]string{}
		if raw, err := callJSON(a.s, "tools", "list", map[string]any{}); err == nil {
			var listed struct {
				Tools []struct {
					Name     string `json:"name"`
					ReadOnly bool   `json:"readOnly"`
					Severity string `json:"severity"`
				} `json:"tools"`
			}
			_ = json.Unmarshal(raw, &listed)
			for _, t := range listed.Tools {
				if t.ReadOnly {
					readOnlyTools[t.Name] = true
				}
				if t.Severity != "" {
					toolSeverity[t.Name] = t.Severity
				}
			}
		}

		type toolOutcome struct {
			content    string
			additional []message
		}
		outcomes := make([]toolOutcome, len(llmOut.ToolCalls))

		// Pass 1: parallel read-only.
		var wg sync.WaitGroup
		for i, tc := range llmOut.ToolCalls {
			if !readOnlyTools[tc.Name] || tc.Name == subagentToolName || tc.Name == todoToolName {
				continue
			}
			wg.Add(1)
			go func(i int, tc toolCall) {
				defer wg.Done()
				_ = a.s.Emit("presentation", "render", marshal(map[string]any{
					"kind": "message_text", "level": "info",
					"text":      "Running " + tc.Name + "…",
					"sessionId": sessionID,
				}))
				out, addCtx, callErr := a.callTool(sessionID, tc, toolSeverity[tc.Name])
				if callErr != nil {
					outcomes[i].content = "error: " + callErr.Error()
				} else {
					outcomes[i].content = out
					outcomes[i].additional = addCtx
				}
			}(i, tc)
		}
		wg.Wait()

		// Pass 2: serial for everything not already finished in parallel.
		for i, tc := range llmOut.ToolCalls {
			parallelDone := readOnlyTools[tc.Name] && tc.Name != subagentToolName && tc.Name != todoToolName
			if !parallelDone {
				var resultContent string
				var additionalContexts []message

				if tc.Name == todoToolName {
					out, err := a.handleTodo(sessionID, tc)
					if err != nil {
						resultContent = "error: " + err.Error()
					} else {
						resultContent = out
					}
				} else if tc.Name == subagentToolName {
					var in struct {
						Input        string   `json:"input"`
						Text         string   `json:"text"`
						SystemPrompt string   `json:"systemPrompt"`
						Mode         string   `json:"mode"`
						Tools        []string `json:"tools"`
					}
					if len(tc.Arguments) > 0 {
						_ = json.Unmarshal(tc.Arguments, &in)
					}
					input := in.Input
					if input == "" {
						input = in.Text
					}
					if subagentUsed >= MaxSubagentsPerTurn {
						resultContent = fmt.Sprintf("error: subagent budget exhausted (max %d per turn). Finish with ordinary tools or a direct answer.", MaxSubagentsPerTurn)
					} else if in.Mode == "async" {
						// Async subagent: child Session runs in the background; result is injected.
						childID := a.nextSubagentID()
						createPayload := map[string]any{
							"sessionId":     childID,
							"parentSession": sessionID,
							"origin":        "subagent",
						}
						if ws := a.sessionWorkspace(sessionID); ws != "" {
							createPayload["workspace"] = ws
						}
						if _, err := callJSON(a.s, "session", "create", createPayload); err != nil {
							resultContent = "error: " + err.Error()
						} else {
							go func(childID, input, sys string, parent string) {
								childRes, cerr := a.runTurn(childID, input, false, sys)
								text := ""
								if cerr != nil {
									text = "error: " + cerr.Error()
								} else if childRes != nil {
									text = childRes.Assistant
								}
								_, _ = callJSON(a.s, "agent", "inject", map[string]any{
									"sessionId": parent,
									"role":      "system",
									"content":   "Async subagent " + childID + " finished:\n" + text,
								})
							}(childID, input, in.SystemPrompt, sessionID)
							subagentUsed++
							resultContent = "async subagent started: " + childID + " (result will be injected when ready)"
						}
					} else if strings.TrimSpace(input) == "" {
						resultContent = "error: subagent input is required"
					} else {
						childID := a.nextSubagentID()
						createPayload := map[string]any{
							"sessionId":     childID,
							"parentSession": sessionID,
							"origin":        "subagent",
						}
						if ws := a.sessionWorkspace(sessionID); ws != "" {
							createPayload["workspace"] = ws
						}
						if _, err := callJSON(a.s, "session", "create", createPayload); err != nil {
							resultContent = "error: " + err.Error()
						} else {
							childRes, cerr := a.runTurn(childID, input, false, in.SystemPrompt)
							subagentUsed++
							if cerr != nil {
								resultContent = "error: " + cerr.Error()
							} else {
								resultContent = childRes.Assistant
							}
						}
					}
				} else {
					_ = a.s.Emit("presentation", "render", marshal(map[string]any{
						"kind": "message_text", "level": "info",
						"text":      "Running " + tc.Name + "…",
						"sessionId": sessionID,
					}))
					out, addCtx, callErr := a.callTool(sessionID, tc, toolSeverity[tc.Name])
					if callErr != nil {
						resultContent = "error: " + callErr.Error()
					} else {
						resultContent = out
						additionalContexts = addCtx
					}
				}
				outcomes[i] = toolOutcome{content: resultContent, additional: additionalContexts}
			}

			resultContent := outcomes[i].content
			additionalContexts := outcomes[i].additional

			_ = a.s.Emit("presentation", "render", marshal(map[string]any{
				"kind": "summary_text", "title": tc.Name,
				"detail": truncate(resultContent, 800), "sessionId": sessionID,
			}))

			if err := a.appendOne(sessionID, map[string]any{
				"type":    "tool_result",
				"role":    "tool",
				"content": resultContent,
				"meta":    map[string]any{"tool_call_id": tc.ID},
			}); err != nil {
				return nil, fmt.Errorf("agent loop: %w", err)
			}
			for _, ac := range additionalContexts {
				role := ac.Role
				if role == "" {
					role = "system"
				}
				if err := a.appendOne(sessionID, map[string]any{
					"type": "message", "role": role, "content": ac.Content,
				}); err != nil {
					return nil, fmt.Errorf("agent loop: %w", err)
				}
			}
		}

		_ = a.appendOne(sessionID, map[string]any{
			"type": "step_end",
			"role": "host",
			"meta": map[string]any{"turn": turnN, "step": stepN, "reason": "tools"},
		})
	}

	msgs, _ := a.deriveMessages(sessionID)
	turnFailed = false
	return &turnResult{
		User:      userInput,
		Assistant: assistant,
		Chunks:    allChunks,
		ToolCalls: toolNames,
		Messages:  msgs,
	}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
