package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsEnvelope(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	got := p.settingsGet()
	if got.Values["base_url"] == "" || got.Values["default_model"] == "" {
		t.Fatalf("values = %#v", got.Values)
	}
	models, ok := got.Values["models"].([]modelConfig)
	if !ok || len(models) == 0 {
		t.Fatalf("models = %#v", got.Values["models"])
	}
	key, ok := got.Values["api_key"].(map[string]any)
	if !ok || key["configured"] != false {
		t.Fatalf("api_key = %#v", got.Values["api_key"])
	}

	var in settingsSaveIn
	in.Values.BaseURL = "https://example.test/v1/"
	in.Values.APIKey = "sk-1234567890"
	in.Values.Models = []modelConfig{
		{ID: "small", Label: "Small", ContextWindow: 8192},
		{ID: "large", Label: "Large", ContextWindow: 131072},
	}
	in.Values.DefaultModel = "large"
	out, err := p.settingsSave(in)
	if err != nil || !out.Saved {
		t.Fatalf("save = %#v, %v", out, err)
	}
	cfg := p.snapshot()
	if cfg.BaseURL != "https://example.test/v1" || cfg.APIKey != "sk-1234567890" {
		t.Fatalf("config = %#v", cfg)
	}
	if cfg.DefaultModel != "large" || len(cfg.Models) != 2 {
		t.Fatalf("models = %#v", cfg)
	}
	persisted := readPersistedConfig(t, filepath.Join(dir, configFile))
	if persisted.APIKey != "sk-1234567890" {
		t.Fatalf("api_key not persisted: %#v", persisted)
	}
	if persisted.DefaultModel != "large" || len(persisted.Models) != 2 {
		t.Fatalf("persisted models = %#v", persisted)
	}
}

func TestSettingsClearAPIKey(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	var in settingsSaveIn
	in.Values.APIKey = "sk-clear-me"
	in.Values.Models = []modelConfig{{ID: "m", Label: "M", ContextWindow: 8192}}
	in.Values.DefaultModel = "m"
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}
	in.Values.APIKey = ""
	in.Values.ClearAPIKey = true
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}
	if p.snapshot().APIKey != "" {
		t.Fatal("api key was not cleared")
	}
	persisted := readPersistedConfig(t, filepath.Join(dir, configFile))
	if persisted.APIKey != "" {
		t.Fatalf("persisted api key = %q", persisted.APIKey)
	}
}

func TestSettingsRejectsInvalidModels(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	var in settingsSaveIn
	in.Values.Models = []modelConfig{{ID: "m"}}
	in.Values.DefaultModel = "missing"
	if _, err := p.settingsSave(in); err == nil {
		t.Fatal("default outside catalog accepted")
	}
	in.Values.Models = nil
	in.Values.DefaultModel = ""
	if _, err := p.settingsSave(in); err == nil {
		t.Fatal("empty catalog accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, configFile)); !os.IsNotExist(err) {
		t.Fatalf("invalid save wrote config: %v", err)
	}
}

func readPersistedConfig(t *testing.T, path string) config {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted config
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	return persisted
}
