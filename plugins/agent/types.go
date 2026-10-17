package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/contracts"
)

// Compatibility aliases keep existing plugin-local names while the shared JSON
// shape remains owned by contracts/core-plugin-contracts.schema.json.
type (
	turnIn         = contracts.TurnInput
	turnOut        = contracts.TurnOutput
	cancelIn       = contracts.CancelInput
	cancelOut      = contracts.CancelOutput
	usage          = contracts.Usage
	chatMessage    = contracts.ChatMessage
	toolCall       = contracts.ToolCall
	toolDefinition = contracts.ToolDefinition
	toolFunction   = contracts.ToolFunction
	llmCompleteIn  = contracts.LLMCompleteInput
	llmCompleteOut = contracts.LLMCompleteOutput
	llmCancelIn    = contracts.LLMCancelInput
	llmCancelOut   = contracts.LLMCancelOutput
	streamMessage  = contracts.StreamMessage
	streamSnapshot = contracts.StreamSnapshot
)

func addUsage(dst *usage, src usage) {
	dst.PromptTokens += src.PromptTokens
	dst.CompletionTokens += src.CompletionTokens
	dst.TotalTokens += src.TotalTokens
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Severity    string          `json:"severity,omitempty"`
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
	SessionID string           `json:"session_id"`
	Facts     []map[string]any `json:"facts"`
}

type sessionAppendOut struct {
	Seqs []int `json:"seqs"`
}

type messagesOut struct {
	Messages []chatMessage `json:"messages"`
}

const (
	RoleAssistant = "assistant"

	schemeChat        = "chat"
	schemeToolCalling = "tool_calling"
	schemeCoding      = "coding"
)
