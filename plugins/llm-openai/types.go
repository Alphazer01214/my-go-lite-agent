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

// modelConfig is one catalog entry: a model plus the OpenAI-compatible
// endpoint that serves it. Each entry carries its own base_url and api_key so
// models from different providers sit side by side in one catalog.
type modelConfig struct {
	ID            string `json:"id"`
	Label         string `json:"label,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
}

// modelSummary is the catalog view for consumers that only pick a model (the
// session dropdown, the agent). Endpoints and keys never leave the plugin on
// llm.models.list.
type modelSummary struct {
	ID            string `json:"id"`
	Label         string `json:"label,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

// modelEntryIn is one catalog entry on an input payload. APIKey is a pointer
// so three states survive decoding: an absent field keeps the key stored under
// the same model id, an empty string clears it, any other value replaces it.
// A masked echo (the value config.get and settings.get hand out) counts as
// "keep", so a caller can round-trip what it read without knowing the key.
type modelEntryIn struct {
	ID            string  `json:"id"`
	Label         string  `json:"label,omitempty"`
	ContextWindow int     `json:"context_window,omitempty"`
	BaseURL       string  `json:"base_url,omitempty"`
	APIKey        *string `json:"api_key,omitempty"`
}

type config struct {
	// BaseURL, APIKey and Model are legacy. They are read from pre-multi-provider
	// config files as defaults for entries that carry no endpoint of their own,
	// and llm.config.get mirrors the default entry into them for old callers.
	// They are not written anymore: saveConfig persists the catalog shape.
	BaseURL      string        `json:"base_url,omitempty"`
	APIKey       string        `json:"api_key,omitempty"`
	Model        string        `json:"model,omitempty"`
	Models       []modelConfig `json:"models,omitempty"`
	DefaultModel string        `json:"default_model,omitempty"`
}

type configGetOut struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	// Models carries the per-model endpoint; api_key entries are masked.
	Models       []modelConfig `json:"models"`
	DefaultModel string        `json:"default_model"`
}

type configSetIn struct {
	// Legacy top-level endpoint fields. With `models` present they fill entries
	// that carry no endpoint of their own; without `models` they update the
	// default model's entry.
	BaseURL      string          `json:"base_url,omitempty"`
	APIKey       string          `json:"api_key,omitempty"`
	Model        string          `json:"model,omitempty"`
	Models       *[]modelEntryIn `json:"models,omitempty"`
	DefaultModel string          `json:"default_model,omitempty"`
}

type modelsListOut struct {
	DefaultModel string         `json:"default_model"`
	Models       []modelSummary `json:"models"`
}

type modelsSetIn struct {
	Models       []modelEntryIn `json:"models"`
	DefaultModel string         `json:"default_model,omitempty"`
}
