package main

import "github.com/tomori/my-go-lite-agent/contracts"

// Compatibility aliases keep plugin-local names while the shared JSON shape is
// defined by contracts/core-plugin-contracts.schema.json.
type (
	chatMessage   = contracts.ChatMessage
	toolCall      = contracts.ToolCall
	tool          = contracts.ToolDefinition
	completeIn    = contracts.LLMCompleteInput
	usage         = contracts.Usage
	completeOut   = contracts.LLMCompleteOutput
	chunkText     = contracts.LLMTextChunk
	chunkToolCall = contracts.LLMToolCallChunk
	llmCancelIn   = contracts.LLMCancelInput
	llmCancelOut  = contracts.LLMCancelOutput
)

type config struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type configGetOut struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type configSetIn struct {
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model,omitempty"`
}
