package main

import (
	"encoding/json"
)

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type llmMeta struct {
	SessionID     string `json:"session_id"`
	TurnID        string `json:"turn_id,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type llmCompleteIn struct {
	CallID      string           `json:"call_id,omitempty"`
	Messages    []chatMessage    `json:"messages"`
	Tools       []toolDefinition `json:"tools,omitempty"`
	Model       string           `json:"model,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      *bool            `json:"stream,omitempty"`
	Meta        *llmMeta         `json:"meta,omitempty"`
}

type llmCompleteOut struct {
	Content       string     `json:"content"`
	Reasoning     string     `json:"reasoning,omitempty"`
	ToolCalls     []toolCall `json:"tool_calls,omitempty"`
	FinishReason  string     `json:"finish_reason"`
	Usage         usage      `json:"usage"`
	Model         string     `json:"model,omitempty"`
	ContextWindow int        `json:"context_window,omitempty"`
}

type llmCancelIn struct {
	CallID string `json:"call_id"`
}

type llmCancelOut struct {
	CallID    string `json:"call_id"`
	Cancelled bool   `json:"cancelled"`
}

type streamMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type streamSnapshot struct {
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	StepID    string        `json:"step_id"`
	UpdateID  int           `json:"update_id"`
	Message   streamMessage `json:"message"`
	Status    string        `json:"status"`
}

type turnIn struct {
	Input           string   `json:"input"`
	SessionID       string   `json:"session_id,omitempty"`
	WorkspaceID     string   `json:"workspace_id,omitempty"`
	Workspace       string   `json:"workspace,omitempty"`
	Model           string   `json:"model,omitempty"`
	ContextWindow   int      `json:"context_window,omitempty"`
	MaxSteps        int      `json:"max_steps,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	DeniedTools     []string `json:"denied_tools,omitempty"`
	ParentSessionID string   `json:"parent_session_id,omitempty"`
	Origin          string   `json:"origin,omitempty"`
	DelegationDepth int      `json:"delegation_depth,omitempty"`
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

func addUsage(dst *usage, src usage) {
	dst.PromptTokens += src.PromptTokens
	dst.CompletionTokens += src.CompletionTokens
	dst.TotalTokens += src.TotalTokens
}

type toolSpec struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	Risk         int             `json:"risk,omitempty"`
	ParallelSafe bool            `json:"parallel_safe,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
}

type config struct {
	DefaultScheme    string `json:"default_scheme"`
	MaxSteps         int    `json:"max_steps"`
	ToolParallelism  string `json:"tool_parallelism,omitempty"`
	MaxParallelTools int    `json:"max_parallel_tools,omitempty"`
}

type configGetOut struct {
	DefaultScheme string `json:"default_scheme"`
	MaxSteps      int    `json:"max_steps"`
}

type configSetIn struct {
	DefaultScheme string `json:"default_scheme,omitempty"`
	MaxSteps      int    `json:"max_steps,omitempty"`
}

// session payloads (subset)
type sessionCreateIn struct {
	SessionID       string `json:"session_id,omitempty"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Name            string `json:"name,omitempty"`
	Model           string `json:"model,omitempty"`
	ContextWindow   int    `json:"context_window,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegation_depth,omitempty"`
}

type sessionCreateOut struct {
	SessionID string `json:"session_id"`
}

type sessionInfoOut struct {
	WorkspaceID   string `json:"workspace_id,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type modelConfig struct {
	ID            string `json:"id"`
	Label         string `json:"label,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type modelsListOut struct {
	DefaultModel string        `json:"default_model"`
	Models       []modelConfig `json:"models"`
}

type sessionAppendIn struct {
	SessionID string           `json:"session_id"`
	Facts     []map[string]any `json:"facts"`
}

type sessionAppendOut struct {
	Seqs []int `json:"seqs"`
}

type messagesOut struct {
	Messages []chatMessage `json:"messages"`
}

type modelContextOut struct {
	Messages      []chatMessage    `json:"messages"`
	Tools         []toolDefinition `json:"tools"`
	ToolSpecs     []toolSpec       `json:"-"`
	TokenEstimate int              `json:"token_estimate"`
	// Degraded is set when the baseline had to omit tools. model-context runs
	// later on the llm.complete chain and replaces the content zone; the agent
	// does not call model-context.build directly.
	Degraded string `json:"degraded,omitempty"`
}

type toolResult struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta"`
}

const (
	RoleAssistant = "assistant"

	schemeChat        = "chat"
	schemeToolCalling = "tool_calling"
)
