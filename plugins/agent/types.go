package main

import "encoding/json"

type turnIn struct {
	Input        string   `json:"input"`
	SessionID    string   `json:"session_id,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
	MaxSteps     int      `json:"max_steps,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u *usage) add(o usage) {
	u.PromptTokens += o.PromptTokens
	u.CompletionTokens += o.CompletionTokens
	u.TotalTokens += o.TotalTokens
}

type turnOut struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
	Steps     int    `json:"steps"`
	Cancelled bool   `json:"cancelled"`
	Usage     usage  `json:"usage"`
	Seqs      []int  `json:"seqs"`
}

type cancelIn struct {
	SessionID string `json:"session_id"`
}

type cancelOut struct {
	SessionID string `json:"session_id"`
	Cancelled bool   `json:"cancelled"`
}

type config struct {
	DefaultScheme string `json:"default_scheme"`
	MaxSteps      int    `json:"max_steps"`
}

type configGetOut struct {
	DefaultScheme string `json:"default_scheme"`
	MaxSteps      int    `json:"max_steps"`
}

type configSetIn struct {
	DefaultScheme string `json:"default_scheme,omitempty"`
	MaxSteps      int    `json:"max_steps,omitempty"`
}

// chatMessage for llm.complete (local copy).
type chatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Severity    string          `json:"severity,omitempty"`
}

type llmCompleteIn struct {
	Messages []chatMessage   `json:"messages"`
	Tools    json.RawMessage `json:"tools,omitempty"`
	Stream   *bool           `json:"stream,omitempty"`
}

type llmCompleteOut struct {
	Content      string     `json:"content"`
	Reasoning    string     `json:"reasoning,omitempty"`
	ToolCalls    []toolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        usage      `json:"usage"`
}

// session payloads (subset)
type sessionCreateIn struct {
	SessionID string `json:"session_id,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type sessionCreateOut struct {
	SessionID string `json:"session_id"`
}

type sessionAppendIn struct {
	SessionID string          `json:"session_id"`
	Facts     []map[string]any `json:"facts"`
}

type sessionAppendOut struct {
	Seqs []int `json:"seqs"`
}

type messagesOut struct {
	Messages []chatMessage `json:"messages"`
}

const (
	schemeChat        = "chat"
	schemeToolCalling = "tool_calling"
	schemeCoding      = "coding"
)
