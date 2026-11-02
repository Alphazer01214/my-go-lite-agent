package main

import (
	"path/filepath"
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
