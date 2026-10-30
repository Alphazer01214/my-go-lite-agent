package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const llmConfigPath = "config.json"

type modelSettings struct {
	ID            string          `json:"id"`
	Label         string          `json:"label,omitempty"`
	ContextWindow int             `json:"context_window,omitempty"`
	BaseURL       string          `json:"base_url"`
	APIKey        json.RawMessage `json:"api_key,omitempty"`
	Streaming     bool            `json:"streaming"`
	Tools         bool            `json:"tools"`
	Vision        bool            `json:"vision"`
	Audio         bool            `json:"audio"`
	Reasoning     bool            `json:"reasoning"`
	JsonSchema    bool            `json:"json_schema"`
}

type llmSettingsValues struct {
	Models       []modelSettings `json:"models"`
	DefaultModel string          `json:"default_model"`
}

type llmSettingsSaveReq struct {
	Values llmSettingsValues `json:"values"`
}

type settingsSaveResp struct {
	Saved bool `json:"saved"`
}

type modelCatalogEntry struct {
	ID            string            `json:"id"`
	Label         string            `json:"label,omitempty"`
	ContextWindow int               `json:"context_window,omitempty"`
	Capabilities  ModelCapabilities `json:"capabilities"`
}

type modelListResp struct {
	DefaultModel string              `json:"default_model,omitempty"`
	Models       []modelCatalogEntry `json:"models"`
}

func saveConfig(path string, cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (p *plugin) snapshotConfig() config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func settingsModelFromConfig(model ModelConfig) map[string]any {
	return map[string]any{
		"id":             model.ID,
		"label":          model.Label,
		"context_window": model.ContextWindow,
		"base_url":       model.BaseURL,
		"api_key":        map[string]any{"configured": model.APIKey != ""},
		"streaming":      model.Capabilities.Streaming,
		"tools":          model.Capabilities.Tools,
		"vision":         model.Capabilities.Vision,
		"audio":          model.Capabilities.Audio,
		"reasoning":      model.Capabilities.Reasoning,
		"json_schema":    model.Capabilities.JsonSchema,
	}
}

func parseSecretUpdate(raw json.RawMessage) (string, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", false, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text == "" {
			return "", false, nil
		}
		return text, true, nil
	}
	var state struct {
		Configured bool `json:"configured"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return "", false, fmt.Errorf("api_key must be a string or configured state")
	}
	return "", false, nil
}

func (p *plugin) handleSettingsGet(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	cfg := p.snapshotConfig()
	models := make([]map[string]any, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		models = append(models, settingsModelFromConfig(model))
	}
	return json.Marshal(map[string]any{
		"values": map[string]any{
			"models":        models,
			"default_model": cfg.DefaultModel,
		},
	})
}

func (p *plugin) handleSettingsSave(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input llmSettingsSaveReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	existing := make(map[string]ModelConfig, len(p.cfg.Models))
	for _, model := range p.cfg.Models {
		existing[model.ID] = model
	}

	cfg := config{DefaultModel: strings.TrimSpace(input.Values.DefaultModel)}
	cfg.Models = make([]ModelConfig, 0, len(input.Values.Models))
	for _, item := range input.Values.Models {
		model := ModelConfig{
			ID:            strings.TrimSpace(item.ID),
			Label:         strings.TrimSpace(item.Label),
			ContextWindow: item.ContextWindow,
			BaseURL:       strings.TrimSpace(item.BaseURL),
			Capabilities: ModelCapabilities{
				Streaming:  item.Streaming,
				Tools:      item.Tools,
				Vision:     item.Vision,
				Audio:      item.Audio,
				Reasoning:  item.Reasoning,
				JsonSchema: item.JsonSchema,
			},
		}
		update, present, err := parseSecretUpdate(item.APIKey)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "model "+model.ID+": "+err.Error())
		}
		if present {
			model.APIKey = update
		} else if old, ok := existing[model.ID]; ok {
			model.APIKey = old.APIKey
		}
		cfg.Models = append(cfg.Models, model)
	}
	if err := cfg.validate(); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if err := saveConfig(llmConfigPath, cfg); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	p.cfg = cfg
	return json.Marshal(settingsSaveResp{Saved: true})
}

func (p *plugin) handleModelsList(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	cfg := p.snapshotConfig()
	models := make([]modelCatalogEntry, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		models = append(models, modelCatalogEntry{
			ID:            model.ID,
			Label:         model.Label,
			ContextWindow: model.ContextWindow,
			Capabilities:  model.Capabilities,
		})
	}
	return json.Marshal(modelListResp{DefaultModel: cfg.DefaultModel, Models: models})
}
