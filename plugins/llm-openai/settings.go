package main

import "strings"

type settingsGetOut struct {
	Values map[string]any `json:"values"`
}

type settingsSaveIn struct {
	Values struct {
		Models       []modelEntryIn `json:"models"`
		DefaultModel string         `json:"default_model,omitempty"`
	} `json:"values"`
}

type settingsSaveOut struct {
	Saved bool `json:"saved"`
}

// settingsGet hands the Settings Renderer one row per model, each with its own
// endpoint. api_key comes back masked: the masked value is the placeholder the
// renderer round-trips, so llm.settings.save can tell "keep the stored key"
// from "clear it" without the plaintext ever leaving the plugin.
func (p *plugin) settingsGet() settingsGetOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return settingsGetOut{Values: map[string]any{
		"models":        maskedModels(p.cfg.Models),
		"default_model": p.cfg.DefaultModel,
	}}
}

func (p *plugin) settingsSave(in settingsSaveIn) (settingsSaveOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	models, err := validateModels(in.Values.Models, p.cfg.Models)
	if err != nil {
		return settingsSaveOut{}, errBadArguments(err.Error())
	}
	def := strings.TrimSpace(in.Values.DefaultModel)
	if def == "" {
		def = models[0].ID
	}
	if !hasModel(models, def) {
		return settingsSaveOut{}, errBadArguments("default_model is not in models")
	}
	p.cfg = normalizeConfig(config{Models: models, DefaultModel: def})
	if err := saveConfig(p.cfg); err != nil {
		return settingsSaveOut{}, errHandler(err.Error())
	}
	return settingsSaveOut{Saved: true}, nil
}
