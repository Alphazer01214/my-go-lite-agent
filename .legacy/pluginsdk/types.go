package pluginsdk

import "encoding/json"

// Model-facing contract types shared by session/loop plugins and Render Media
// paint (ADR-0030). Host/Medium Go runtime must not orchestrate these; Medium
// may import them for display (P2).

// Message is one model-visible chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// TurnResult is one Agent Loop turn returned by the loop provider (ADR-0016).
type TurnResult struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Chunks    []string  `json:"chunks"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
	Messages  []Message `json:"messages"`
}
