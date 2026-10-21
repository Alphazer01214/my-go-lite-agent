package main

import "strings"

type settingsGetOut struct {
	Values map[string]any `json:"values"`
}

type settingsSaveIn struct {
	Values struct {
		BaseURL      string        `json:"base_url,omitempty"`
		APIKey       string        `json:"api_key,omitempty"`
		ClearAPIKey  bool          `json:"clear_api_key,omitempty"`
		Models       []modelConfig `json:"models"`
		DefaultModel string        `json:"default_model,omitempty"`
	} `json:"values"`
}

type settingsSaveOut struct {
	Saved bool `json:"saved"`
}

func (p *plugin) settingsGet() settingsGetOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return settingsGetOut{Values: map[string]any{
		"base_url":      p.cfg.BaseURL,
		"api_key":       map[string]any{"configured": p.cfg.APIKey != ""},
		"clear_api_key": false,
		"models":        append([]modelConfig(nil), p.cfg.Models...),
		"default_model": p.cfg.DefaultModel,
	}}
}

func (p *plugin) settingsSave(in settingsSaveIn) (settingsSaveOut, error) {
	models, err := validateModels(in.Values.Models)
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if in.Values.BaseURL != "" {
		p.cfg.BaseURL = normalizeBaseURL(in.Values.BaseURL)
	}
	if in.Values.ClearAPIKey {
		p.cfg.APIKey = ""
	} else if in.Values.APIKey != "" {
		p.cfg.APIKey = in.Values.APIKey
	}
	p.cfg.Models = models
	p.cfg.DefaultModel = def
	p.cfg.Model = def
	if err := saveConfig(p.cfg); err != nil {
		return settingsSaveOut{}, errHandler(err.Error())
	}
	return settingsSaveOut{Saved: true}, nil
}
