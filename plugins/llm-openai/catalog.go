package main

import (
	"fmt"
	"strings"
)

func sanitizeModels(in []modelConfig, legacyBase, legacyKey string) []modelConfig {
	out := make([]modelConfig, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, model := range in {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || seen[model.ID] {
			continue
		}
		model.Label = strings.TrimSpace(model.Label)
		if model.Label == "" {
			model.Label = model.ID
		}
		if model.ContextWindow < 0 {
			model.ContextWindow = 0
		}
		model.BaseURL = normalizeBaseURL(model.BaseURL)
		if model.BaseURL == "" {
			model.BaseURL = legacyBase
		}
		if model.BaseURL == "" {
			model.BaseURL = defaultBaseURL
		}
		model.APIKey = strings.TrimSpace(model.APIKey)
		if model.APIKey == "" {
			model.APIKey = strings.TrimSpace(legacyKey)
		}
		seen[model.ID] = true
		out = append(out, model)
	}
	return out
}

func validateModels(in []modelEntryIn, old []modelConfig) ([]modelConfig, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("models must not be empty")
	}
	out := make([]modelConfig, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, entry := range in {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			return nil, fmt.Errorf("model id is required")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate model id %q", id)
		}
		if entry.ContextWindow < 0 {
			return nil, fmt.Errorf("model %q context_window must not be negative", id)
		}
		label := strings.TrimSpace(entry.Label)
		if label == "" {
			label = id
		}
		base := normalizeBaseURL(entry.BaseURL)
		if base == "" {
			if prev := findModel(old, id); prev != nil {
				base = prev.BaseURL
			}
		}
		if base == "" {
			return nil, fmt.Errorf("model %q base_url is required", id)
		}
		seen[id] = true
		out = append(out, modelConfig{
			ID:            id,
			Label:         label,
			ContextWindow: entry.ContextWindow,
			BaseURL:       base,
			APIKey:        resolveAPIKey(entry.APIKey, id, old),
		})
	}
	return out, nil
}

func findModel(models []modelConfig, id string) *modelConfig {
	for i := range models {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

func hasModel(models []modelConfig, id string) bool {
	return findModel(models, id) != nil
}

func (p *plugin) modelsList() modelsListOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.modelsListLocked()
}

func (p *plugin) modelsListLocked() modelsListOut {
	models := make([]modelSummary, 0, len(p.cfg.Models))
	for _, model := range p.cfg.Models {
		models = append(models, modelSummary{
			ID:            model.ID,
			Label:         model.Label,
			ContextWindow: model.ContextWindow,
		})
	}
	return modelsListOut{DefaultModel: p.cfg.DefaultModel, Models: models}
}

func (p *plugin) modelsSet(in modelsSetIn) (modelsListOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	models, err := validateModels(in.Models, p.cfg.Models)
	if err != nil {
		return modelsListOut{}, errBadArguments(err.Error())
	}
	def := strings.TrimSpace(in.DefaultModel)
	if def == "" {
		def = models[0].ID
	}
	if !hasModel(models, def) {
		return modelsListOut{}, errBadArguments("default_model is not in models")
	}
	// A no-op save must not rewrite config.json unnecessarily.
	if p.cfg.DefaultModel == def && sameModels(p.cfg.Models, models) {
		return p.modelsListLocked(), nil
	}
	p.cfg = normalizeConfig(config{Models: models, DefaultModel: def})
	if err := saveConfig(p.cfg); err != nil {
		return modelsListOut{}, errHandler(err.Error())
	}
	return p.modelsListLocked(), nil
}

func sameModels(a, b []modelConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func inheritEndpoint(in []modelEntryIn, baseURL, apiKey string) []modelEntryIn {
	baseURL = normalizeBaseURL(baseURL)
	key := strings.TrimSpace(apiKey)
	if baseURL == "" && key == "" {
		return in
	}
	out := append([]modelEntryIn(nil), in...)
	for i := range out {
		if out[i].BaseURL == "" {
			out[i].BaseURL = baseURL
		}
		if out[i].APIKey == nil && key != "" {
			out[i].APIKey = &key
		}
	}
	return out
}
