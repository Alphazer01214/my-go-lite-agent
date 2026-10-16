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
	if got.Values["default_scheme"] == "" || got.Values["max_steps"] == 0 {
		t.Fatalf("values = %#v", got.Values)
	}

	var in settingsSaveIn
	in.Values.DefaultScheme = "coding"
	in.Values.MaxSteps = 24
	out, err := p.settingsSave(in)
	if err != nil || !out.Saved {
		t.Fatalf("save = %#v, %v", out, err)
	}
	if p.snapshot().DefaultScheme != "coding" || p.snapshot().MaxSteps != 24 {
		t.Fatalf("config = %#v", p.snapshot())
	}
}
