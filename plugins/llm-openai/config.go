package main

import (
	"encoding/json"
	"os"
	"strings"
)

const (
	configFile     = "config.json"
	defaultBaseURL = "https://api.deepseek.com/v1"
	defaultModelID = "deepseek-chat"
)

func loadConfig() config {
	file := config{
		Model:        defaultModelID,
		DefaultModel: defaultModelID,
		Models:       []modelConfig{{ID: defaultModelID, Label: defaultModelID}},
	}
	if data, err := os.ReadFile(configFile); err == nil {
		var c config
		if json.Unmarshal(data, &c) == nil {
			// The file wins over the built-in defaults wholesale; normalizeConfig
			// fills whatever it left empty.
			file = c
			if file.Model == "" {
				file.Model = defaultModelID
			}
		}
	}
	cfg := normalizeConfig(file)

	// env wins at startup. One base URL and one key cannot describe several
	// providers, so they address the default model's entry only.
	models := append([]modelConfig(nil), cfg.Models...)
	def := cfg.DefaultModel
	if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
		def = v
		if !hasModel(models, v) {
			models = append(models, modelConfig{ID: v, Label: v})
		}
	}
	if entry := findModel(models, def); entry != nil {
		if v := normalizeBaseURL(os.Getenv("OPENAI_BASE_URL")); v != "" {
			entry.BaseURL = v
		}
		if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
			entry.APIKey = v
		}
	}
	return normalizeConfig(config{Models: models, DefaultModel: def})
}

func normalizeConfig(c config) config {
	legacyBase := normalizeBaseURL(c.BaseURL)
	if legacyBase == "" {
		legacyBase = defaultBaseURL
	}
	models := sanitizeModels(c.Models, legacyBase, strings.TrimSpace(c.APIKey))
	legacy := strings.TrimSpace(c.Model)
	if len(models) == 0 {
		id := legacy
		if id == "" {
			id = defaultModelID
		}
		models = []modelConfig{{ID: id, Label: id, BaseURL: legacyBase, APIKey: strings.TrimSpace(c.APIKey)}}
	}
	def := strings.TrimSpace(c.DefaultModel)
	if def == "" && hasModel(models, legacy) {
		def = legacy
	}
	if def == "" || !hasModel(models, def) {
		def = models[0].ID
	}
	entry := findModel(models, def)
	c.Models = models
	c.DefaultModel = def
	c.Model = def
	c.BaseURL = entry.BaseURL
	c.APIKey = entry.APIKey
	return c
}

func (p *plugin) snapshot() config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *plugin) getConfig() configGetOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getConfigLocked()
}

func (p *plugin) configSet(in configSetIn) (configGetOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.applyLocked(in); err != nil {
		return configGetOut{}, errHandler(err.Error())
	}
	return p.getConfigLocked(), nil
}

func (p *plugin) getConfigLocked() configGetOut {
	return configGetOut{
		BaseURL:      p.cfg.BaseURL,
		APIKey:       maskKey(p.cfg.APIKey),
		Model:        p.cfg.DefaultModel,
		Models:       maskedModels(p.cfg.Models),
		DefaultModel: p.cfg.DefaultModel,
	}
}

func (p *plugin) applyLocked(in configSetIn) error {
	old := p.cfg.Models
	models := append([]modelConfig(nil), old...)
	if in.Models != nil {
		normalized, err := validateModels(inheritEndpoint(*in.Models, in.BaseURL, in.APIKey), old)
		if err != nil {
			return err
		}
		models = normalized
	}
	candidate := strings.TrimSpace(in.DefaultModel)
	if candidate == "" {
		candidate = strings.TrimSpace(in.Model)
	}
	if candidate == "" {
		candidate = p.cfg.DefaultModel
	}
	if !hasModel(models, candidate) {
		// A default outside the catalog becomes a new entry that inherits the
		// previous default's endpoint, so `config.set model=x` keeps working.
		entry := modelConfig{
			ID:      candidate,
			Label:   candidate,
			BaseURL: normalizeBaseURL(in.BaseURL),
			APIKey:  strings.TrimSpace(in.APIKey),
		}
		if entry.BaseURL == "" || entry.APIKey == "" {
			if prev := findModel(old, p.cfg.DefaultModel); prev != nil {
				if entry.BaseURL == "" {
					entry.BaseURL = prev.BaseURL
				}
				if entry.APIKey == "" {
					entry.APIKey = prev.APIKey
				}
			}
		}
		models = append(models, entry)
	} else if in.Models == nil && (in.BaseURL != "" || strings.TrimSpace(in.APIKey) != "") {
		// Legacy shape: one endpoint for the catalog is expressed as an update
		// to the default model's entry.
		entry := findModel(models, candidate)
		if in.BaseURL != "" {
			entry.BaseURL = normalizeBaseURL(in.BaseURL)
		}
		if key := strings.TrimSpace(in.APIKey); key != "" {
			entry.APIKey = key
		}
	}
	p.cfg = normalizeConfig(config{Models: models, DefaultModel: candidate})
	return saveConfig(p.cfg)
}

func saveConfig(c config) error {
	persisted := config{
		Models:       sanitizeModels(c.Models, "", ""),
		DefaultModel: c.DefaultModel,
	}
	if len(persisted.Models) == 0 {
		id := strings.TrimSpace(persisted.DefaultModel)
		if id == "" {
			id = defaultModelID
		}
		persisted.Models = []modelConfig{{ID: id, Label: id, BaseURL: defaultBaseURL}}
	}
	if persisted.DefaultModel == "" || !hasModel(persisted.Models, persisted.DefaultModel) {
		persisted.DefaultModel = persisted.Models[0].ID
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0o644)
}
