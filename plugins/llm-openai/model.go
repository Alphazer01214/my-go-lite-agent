package main

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const codeLLMConfigInvalid = "llm_config_invalid"

const (
	defaultRequestTimeoutMS  = 15 * 60 * 1000
	defaultStreamIdleTimeout = 2 * 60 * 1000
	defaultMaxResponseBytes  = 4 * 1024 * 1024
)

type ModelConfig struct {
	ID            string            `json:"id"`
	Label         string            `json:"label,omitempty"`
	ContextWindow int               `json:"context_window,omitempty"`
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Capabilities  ModelCapabilities `json:"capabilities"`
}

type ModelCapabilities struct {
	Vision bool `json:"vision"`
}

type config struct {
	Models              []ModelConfig `json:"models"`
	DefaultModel        string        `json:"default_model"`
	RequestTimeoutMS    int           `json:"request_timeout_ms,omitempty"`
	StreamIdleTimeoutMS int           `json:"stream_idle_timeout_ms,omitempty"`
	MaxResponseBytes    int           `json:"max_response_bytes,omitempty"`
}

func (c config) requestTimeout() time.Duration {
	ms := c.RequestTimeoutMS
	if ms <= 0 {
		ms = defaultRequestTimeoutMS
	}
	return time.Duration(ms) * time.Millisecond
}

func (c config) streamIdleTimeout() time.Duration {
	ms := c.StreamIdleTimeoutMS
	if ms <= 0 {
		ms = defaultStreamIdleTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func (c config) maxResponseBytes() int64 {
	value := c.MaxResponseBytes
	if value <= 0 {
		value = defaultMaxResponseBytes
	}
	return int64(value)
}

func (c config) validate() error {
	if len(c.Models) == 0 {
		return pluginsdk.ErrCode(codeLLMConfigInvalid, "models must be non-empty")
	}
	seen := make(map[string]bool, len(c.Models))
	for _, model := range c.Models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			return pluginsdk.ErrCode(codeLLMConfigInvalid, "model id is required")
		}
		if seen[id] {
			return pluginsdk.ErrCode(codeLLMConfigInvalid, "duplicate model id: "+id)
		}
		seen[id] = true
		if strings.TrimSpace(model.BaseURL) == "" {
			return pluginsdk.ErrCode(codeLLMConfigInvalid, "model "+id+" has no base_url")
		}
		if model.ContextWindow < 0 {
			return pluginsdk.ErrCode(codeLLMConfigInvalid, "model "+id+" has negative context_window")
		}
		if model.MaxTokens < 0 {
			return pluginsdk.ErrCode(codeLLMConfigInvalid, "model "+id+" has negative max_tokens")
		}
	}
	if c.RequestTimeoutMS < 0 {
		return pluginsdk.ErrCode(codeLLMConfigInvalid, "request_timeout_ms must be non-negative")
	}
	if c.StreamIdleTimeoutMS < 0 {
		return pluginsdk.ErrCode(codeLLMConfigInvalid, "stream_idle_timeout_ms must be non-negative")
	}
	if c.MaxResponseBytes < 0 {
		return pluginsdk.ErrCode(codeLLMConfigInvalid, "max_response_bytes must be non-negative")
	}
	if strings.TrimSpace(c.DefaultModel) == "" {
		return pluginsdk.ErrCode(codeLLMConfigInvalid, "default_model is required")
	}
	if !seen[c.DefaultModel] {
		return pluginsdk.ErrCode(codeLLMConfigInvalid, "unknown default_model: "+c.DefaultModel)
	}
	return nil
}

func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, err
	}
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (c config) model(id string) (ModelConfig, error) {
	if id == "" {
		id = c.DefaultModel
	}
	if id == "" && len(c.Models) > 0 {
		id = c.Models[0].ID
	}
	for _, m := range c.Models {
		if m.ID != id {
			continue
		}
		if strings.TrimSpace(m.BaseURL) == "" {
			return ModelConfig{}, pluginsdk.ErrCode(codeLLMConfigInvalid, "model "+id+" has no base_url")
		}
		return m, nil
	}
	if len(c.Models) == 0 {
		return ModelConfig{}, pluginsdk.ErrCode(codeLLMConfigInvalid, "no models configured; edit config.json")
	}
	return ModelConfig{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "unknown model: "+id)
}
