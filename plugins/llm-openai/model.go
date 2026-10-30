package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const codeLLMConfigInvalid = "llm_config_invalid"

type ModelConfig struct {
	ID            string `json:"id"`
	Label         string `json:"label,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key,omitempty"`
}

type ModelCapabilities struct {
	Streaming  bool `json:"streaming"`
	Tools      bool `json:"tools"`
	Vision     bool `json:"vision"`
	Audio      bool `json:"audio"`
	Reasoning  bool `json:"reasoning"`
	JsonSchema bool `json:"json_schema"`
}

type config struct {
	Models       []ModelConfig `json:"models"`
	DefaultModel string        `json:"default_model"`
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
