package main

import (
	"os"
	"testing"
)

func TestSettingsEnvelope(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	got := p.settingsGet()
	if got.Values["base_url"] == "" || got.Values["model"] == "" {
		t.Fatalf("values = %#v", got.Values)
	}
	key, ok := got.Values["api_key"].(map[string]any)
	if !ok || key["configured"] != false {
		t.Fatalf("api_key = %#v", got.Values["api_key"])
	}

	var in settingsSaveIn
	in.Values.BaseURL = "https://example.test/v1/"
	in.Values.Model = "test-model"
	in.Values.APIKey = "sk-1234567890"
	out, err := p.settingsSave(in)
	if err != nil || !out.Saved {
		t.Fatalf("save = %#v, %v", out, err)
	}
	if p.snapshot().BaseURL != "https://example.test/v1" || p.snapshot().APIKey != "sk-1234567890" {
		t.Fatalf("config = %#v", p.snapshot())
	}
}
