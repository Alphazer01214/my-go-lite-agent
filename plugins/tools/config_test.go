package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigNormalize(t *testing.T) {
	cfg := config{
		Capabilities:  []string{"file", " file ", "bad Cap", "host", "tool", "file", ""},
		RefreshTTLMS:  -1,
		InfoTimeoutMS: 0,
	}
	diagnostics := cfg.normalize()

	if len(cfg.Capabilities) != 1 || cfg.Capabilities[0] != "file" {
		t.Fatalf("capabilities = %v, want [file]", cfg.Capabilities)
	}
	if cfg.RefreshTTLMS != defaultRefreshTTLMS {
		t.Fatalf("refresh ttl = %d, want %d", cfg.RefreshTTLMS, defaultRefreshTTLMS)
	}
	if cfg.InfoTimeoutMS != defaultInfoTimeoutMS {
		t.Fatalf("info timeout = %d, want %d", cfg.InfoTimeoutMS, defaultInfoTimeoutMS)
	}
	if len(diagnostics) != 6 {
		t.Fatalf("diagnostics = %d, want 6: %+v", len(diagnostics), diagnostics)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, diagnostics, err := loadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.Capabilities) != 0 {
		t.Fatalf("capabilities = %v, want empty", cfg.Capabilities)
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != "config_missing" {
		t.Fatalf("diagnostics = %+v, want config_missing", diagnostics)
	}
}

func TestLoadConfigMigratesLegacyFileOnlyDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{
  "capabilities": ["file"],
  "refresh_ttl_ms": 2000,
  "info_timeout_ms": 1500
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, diagnostics, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v, want none", diagnostics)
	}
	want := []string{"file", "shell", "python", "skill", "mcp", "subagent"}
	if !reflect.DeepEqual(cfg.Capabilities, want) {
		t.Fatalf("capabilities = %v, want %v", cfg.Capabilities, want)
	}
}

func TestLoadConfigKeepsCustomFileOnlyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	custom := `{
  "capabilities": ["file"],
  "refresh_ttl_ms": 1234,
  "info_timeout_ms": 1500
}`
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, diagnostics, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v, want none", diagnostics)
	}
	if !reflect.DeepEqual(cfg.Capabilities, []string{"file"}) {
		t.Fatalf("capabilities = %v, want [file]", cfg.Capabilities)
	}
	if cfg.RefreshTTLMS != 1234 {
		t.Fatalf("refresh ttl = %d, want 1234", cfg.RefreshTTLMS)
	}
}
