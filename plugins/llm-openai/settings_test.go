package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsEnvelope(t *testing.T) {
	dir := chdirTemp(t)

	p := newPlugin()
	got := p.settingsGet()
	if got.Values["default_model"] == "" {
		t.Fatalf("values = %#v", got.Values)
	}
	models, ok := got.Values["models"].([]modelConfig)
	if !ok || len(models) == 0 {
		t.Fatalf("models = %#v", got.Values["models"])
	}

	var in settingsSaveIn
	in.Values.Models = []modelEntryIn{
		{ID: "small", Label: "Small", ContextWindow: 8192, BaseURL: "https://example.test/v1/", APIKey: keyPtr("sk-1234567890")},
		{ID: "large", Label: "Large", ContextWindow: 131072, BaseURL: "https://other.test/v1"},
	}
	in.Values.DefaultModel = "large"
	out, err := p.settingsSave(in)
	if err != nil || !out.Saved {
		t.Fatalf("save = %#v, %v", out, err)
	}
	cfg := p.snapshot()
	small := findModel(cfg.Models, "small")
	large := findModel(cfg.Models, "large")
	if small.BaseURL != "https://example.test/v1" || small.APIKey != "sk-1234567890" {
		t.Fatalf("small = %+v", small)
	}
	if large.BaseURL != "https://other.test/v1" || large.APIKey != "" {
		t.Fatalf("large = %+v", large)
	}
	if cfg.DefaultModel != "large" || len(cfg.Models) != 2 {
		t.Fatalf("models = %#v", cfg)
	}

	// The renderer sees rows with masked keys, never plaintext.
	rows := p.settingsGet().Values["models"].([]modelConfig)
	if rows[0].APIKey != "sk-***890" || rows[1].APIKey != "" {
		t.Fatalf("masked rows = %#v", rows)
	}

	persisted := readPersistedConfig(t, filepath.Join(dir, configFile))
	if len(persisted.Models) != 2 || persisted.Models[0].APIKey != "sk-1234567890" {
		t.Fatalf("api_key not persisted: %#v", persisted)
	}
	if persisted.DefaultModel != "large" {
		t.Fatalf("persisted models = %#v", persisted)
	}
}

// The settings renderer round-trips what settings.get returned. A masked echo
// keeps the stored key; clearing the row clears the key.
func TestSettingsKeyRoundTrip(t *testing.T) {
	dir := chdirTemp(t)

	p := newPlugin()
	var in settingsSaveIn
	in.Values.Models = []modelEntryIn{
		{ID: "m", BaseURL: "https://m.test/v1", APIKey: keyPtr("sk-clear-me")},
	}
	in.Values.DefaultModel = "m"
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}

	masked := p.settingsGet().Values["models"].([]modelConfig)[0].APIKey
	in.Values.Models[0].APIKey = keyPtr(masked)
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}
	if got := findModel(p.snapshot().Models, "m").APIKey; got != "sk-clear-me" {
		t.Fatalf("masked round-trip lost the key: %q", got)
	}

	in.Values.Models[0].APIKey = keyPtr("")
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}
	if got := findModel(p.snapshot().Models, "m").APIKey; got != "" {
		t.Fatalf("api key was not cleared: %q", got)
	}
	persisted := readPersistedConfig(t, filepath.Join(dir, configFile))
	if len(persisted.Models) != 1 || persisted.Models[0].APIKey != "" {
		t.Fatalf("persisted api key = %#v", persisted)
	}
}

// Renaming a model id keeps its key: the masked echo still matches the key it
// was made from.
func TestSettingsRenameKeepsKey(t *testing.T) {
	chdirTemp(t)

	p := newPlugin()
	var in settingsSaveIn
	in.Values.Models = []modelEntryIn{
		{ID: "old", BaseURL: "https://m.test/v1", APIKey: keyPtr("sk-rename-me")},
	}
	in.Values.DefaultModel = "old"
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}
	masked := p.settingsGet().Values["models"].([]modelConfig)[0].APIKey
	in.Values.Models = []modelEntryIn{
		{ID: "new", BaseURL: "https://m.test/v1", APIKey: keyPtr(masked)},
	}
	in.Values.DefaultModel = "new"
	if _, err := p.settingsSave(in); err != nil {
		t.Fatal(err)
	}
	if got := findModel(p.snapshot().Models, "new").APIKey; got != "sk-rename-me" {
		t.Fatalf("rename lost the key: %q", got)
	}
}

func TestSettingsRejectsInvalidModels(t *testing.T) {
	dir := chdirTemp(t)

	p := newPlugin()
	var in settingsSaveIn
	in.Values.Models = []modelEntryIn{{ID: "m", BaseURL: "https://m.test/v1"}}
	in.Values.DefaultModel = "missing"
	if _, err := p.settingsSave(in); err == nil {
		t.Fatal("default outside catalog accepted")
	}
	in.Values.Models = nil
	in.Values.DefaultModel = ""
	if _, err := p.settingsSave(in); err == nil {
		t.Fatal("empty catalog accepted")
	}
	in.Values.Models = []modelEntryIn{{ID: "m"}}
	if _, err := p.settingsSave(in); err == nil {
		t.Fatal("entry without endpoint accepted")
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
