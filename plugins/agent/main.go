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

const subagentToolName = "run_subagent"

var subagentSchema = map[string]any{
	"name":        subagentToolName,
	"description": "Run a focused subagent on a child session linked to this session and return its final reply.",
	"input_schema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":        map[string]any{"type": "string"},
			"text":         map[string]any{"type": "string"},
			"systemPrompt": map[string]any{"type": "string"},
			"tools":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"mode":         map[string]any{"type": "string", "enum": []string{"sync", "async"}},
		},
		"required": []string{"input"},
	},
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
		return out.Tools, true
	}
	_ = json.Unmarshal(raw, &out)
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

func (a *agent) prepareContext(sessionID string, msgs []message, contextWindow int) ([]message, []toolSchema, error) {
	payload := map[string]any{"messages": msgs}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	if contextWindow > 0 {
		payload["contextWindow"] = contextWindow
	}
	raw, err := callJSON(a.s, "context", "prepare", payload)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Messages []message    `json:"messages"`
		Tools    []toolSchema `json:"tools"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Messages, out.Tools, nil
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

func (a *agent) callTool(sessionID string, tc toolCall) (string, []message, error) {
	args := tc.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	payload := map[string]any{
		"name":      tc.Name,
		"arguments": json.RawMessage(args),
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

	_ = a.s.Serve()
}

func (a *agent) runTurn(sessionID, userInput string, allowSubagent bool, extraSystem string) (*turnResult, error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, &protocol.FrameError{Code: "bad_payload", Message: "user input is required"}
	}
	sessionID = normalizeID(sessionID)
	// Entry turns clear any stale cancel from a previous turn on this Session.
	// Nested subagent turns share the parent's cancel only via Host.

	turnN := a.nextTurnNumber(sessionID)
	if err := a.appendOne(sessionID, map[string]any{
		"type": "turn_start",
		"role": "host",
		"meta": map[string]any{"turn": turnN},
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
	if extraSystem != "" {
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += extraSystem
	}

	schemas, toolsOK := a.collectToolSchemas()
	hasTools := toolsOK
	if !hasTools {
		note := "No tools are mounted in this assembly. Do not claim to use tools, browse the workspace, or run commands. Answer from the conversation only, or ask the user to use the agent assembly if they need file tools."
		if sysText != "" {
			sysText += "\n\n"
		}
		sysText += note
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

	if allowSubagent && hasTools {
		schemas = append(schemas, toolSchema{
			Name:        subagentToolName,
			Description: "Run a focused subagent on a child session linked to this session and return its final reply.",
			InputSchema: marshal(subagentSchema["input_schema"]),
		})
	}

	contextWindow := a.llmInfoContextWindow()
	var allChunks []string
	var toolNames []string
	var assistant string

	for step := 0; step < MaxSteps; step++ {
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
			prepMsgs, prepTools, perr := a.prepareContext(sessionID, arMsgs, contextWindow)
			if perr == nil {
				if len(prepMsgs) > 0 && !messagesEqual(prepMsgs, arMsgs) {
					return nil, &protocol.FrameError{
						Code:    "session_invariant_violation",
						Message: "context.prepare messages are not reconstructable from session log",
					}
				}
				if len(prepTools) > 0 {
					schemas = prepTools
					if allowSubagent && hasTools {
						schemas = append(schemas, toolSchema{
							Name:        subagentToolName,
							Description: "Run a focused subagent on a child session linked to this session and return its final reply.",
							InputSchema: marshal(subagentSchema["input_schema"]),
						})
					}
				}
			}
		} else {
			// First successful prepare marks the provider present.
			if _, _, perr := a.prepareContext(sessionID, arMsgs, contextWindow); perr == nil {
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

		for _, tc := range llmOut.ToolCalls {
			var resultContent string
			var additionalContexts []message

			if tc.Name == subagentToolName {
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
				if in.Mode == "async" {
					resultContent = "error: async subagent is not supported in v1"
				} else if strings.TrimSpace(input) == "" {
					resultContent = "error: subagent input is required"
				} else {
					childID := a.nextSubagentID()
					if _, err := callJSON(a.s, "session", "create", map[string]any{
						"sessionId":     childID,
						"parentSession": sessionID,
						"origin":        "subagent",
					}); err != nil {
						resultContent = "error: " + err.Error()
					} else {
						// Nested turn: in-process (same loop policy), no run_subagent.
						// Host entry lock already covers the parent Session.
						childRes, cerr := a.runTurn(childID, input, false, in.SystemPrompt)
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
				out, addCtx, callErr := a.callTool(sessionID, tc)
				if callErr != nil {
					resultContent = "error: " + callErr.Error()
				} else {
					resultContent = out
					additionalContexts = addCtx
				}
			}

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
