package main

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
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type tool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type completeIn struct {
	CallID      string        `json:"call_id,omitempty"`
	Messages    []chatMessage `json:"messages"`
	Tools       []tool        `json:"tools,omitempty"`
	Model       string        `json:"model,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      *bool         `json:"stream,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type completeOut struct {
	Content      string     `json:"content"`
	Reasoning    string     `json:"reasoning,omitempty"`
	ToolCalls    []toolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        usage      `json:"usage"`
	// Model and ContextWindow describe what actually ran. The caller records
	// them so session audit does not have to guess the configured default.
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type chunkText struct {
	Channel string `json:"channel"`
	Delta   string `json:"delta"`
}

type chunkToolCall struct {
	Channel  string       `json:"channel"`
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function functionCall `json:"function"`
}

type llmCancelIn struct {
	CallID string `json:"call_id"`
}

type llmCancelOut struct {
	CallID    string `json:"call_id"`
	Cancelled bool   `json:"cancelled"`
}

type modelConfig struct {
	ID            string `json:"id"`
	Label         string `json:"label,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type config struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	// Model is the legacy single-model field. It always mirrors DefaultModel
	// at runtime and is still written so older config files stay readable.
	Model        string        `json:"model,omitempty"`
	Models       []modelConfig `json:"models,omitempty"`
	DefaultModel string        `json:"default_model,omitempty"`
}

type configGetOut struct {
	BaseURL      string        `json:"base_url"`
	APIKey       string        `json:"api_key"`
	Model        string        `json:"model"`
	Models       []modelConfig `json:"models"`
	DefaultModel string        `json:"default_model"`
}

type configSetIn struct {
	BaseURL      string         `json:"base_url,omitempty"`
	APIKey       string         `json:"api_key,omitempty"`
	Model        string         `json:"model,omitempty"`
	Models       *[]modelConfig `json:"models,omitempty"`
	DefaultModel string         `json:"default_model,omitempty"`
}

type modelsListOut struct {
	DefaultModel string        `json:"default_model"`
	Models       []modelConfig `json:"models"`
}

type modelsSetIn struct {
	Models       []modelConfig `json:"models"`
	DefaultModel string        `json:"default_model,omitempty"`
}
