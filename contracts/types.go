// Package contracts defines the normative cross-plugin JSON shapes.
// The JSON Schema in this directory is the source of truth. Go types and the
// aliases retained inside plugins are compatibility adapters.
package contracts

import "encoding/json"

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type LLMCompleteInput struct {
	CallID      string           `json:"call_id,omitempty"`
	Messages    []ChatMessage    `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Model       string           `json:"model,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      *bool            `json:"stream,omitempty"`
}

type LLMCompleteOutput struct {
	Content      string     `json:"content"`
	Reasoning    string     `json:"reasoning,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        Usage      `json:"usage"`
}

type LLMTextChunk struct {
	Channel string `json:"channel"`
	Delta   string `json:"delta"`
}

type LLMToolCallChunk struct {
	Channel  string       `json:"channel"`
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

type StreamMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type StreamSnapshot struct {
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	StepID    string        `json:"step_id"`
	UpdateID  int           `json:"update_id"`
	Message   StreamMessage `json:"message"`
	Status    string        `json:"status"`
}

type TurnInput struct {
	Input        string   `json:"input"`
	SessionID    string   `json:"session_id,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
	MaxSteps     int      `json:"max_steps,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

type TurnOutput struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
	Steps     int    `json:"steps"`
	Cancelled bool   `json:"cancelled"`
	Usage     Usage  `json:"usage"`
	Seqs      []int  `json:"seqs"`
}

type CancelInput struct {
	SessionID string `json:"session_id"`
}

type CancelOutput struct {
	SessionID string `json:"session_id"`
	Cancelled bool   `json:"cancelled"`
}

type LLMCancelInput struct {
	CallID string `json:"call_id"`
}

type LLMCancelOutput struct {
	CallID    string `json:"call_id"`
	Cancelled bool   `json:"cancelled"`
}
