package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadManifestReportsUnknownFieldAndContinues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	body := `{
		"name": "demo",
		"version": "0.1.0",
		"protocol": 1,
		"entry": "demo",
		"description": "demo plugin",
		"future_field": true
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, diagnostics := LoadManifest(path)
	if got.Name != "demo" || got.Entry != "demo" || got.Description != "demo plugin" {
		t.Fatalf("manifest = %#v", got)
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != "manifest_unknown_field" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestLoadManifestReadFailureIsDiagnostic(t *testing.T) {
	got, diagnostics := LoadManifest(filepath.Join(t.TempDir(), "missing.json"))
	if got.Name != "" || got.Entry != "" {
		t.Fatalf("manifest = %#v", got)
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != "manifest_read_failed" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestScanRootContinuesAfterInvalidManifest(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "bad"), `{`)
	writeManifest(t, filepath.Join(root, "good"), `{
		"name": "good",
		"version": "0.1.0",
		"protocol": 1,
		"entry": "good"
	}`)

	got := ScanRoot(root)
	if len(got.Plugins) != 1 || got.Plugins[0].Manifest.Name != "good" {
		t.Fatalf("plugins = %#v", got.Plugins)
	}
	if !hasDiagnostic(got.Diagnostics, "manifest_invalid_json") {
		t.Fatalf("diagnostics = %#v", got.Diagnostics)
	}
}

func TestScanRootNameMismatchOnlyReports(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "folder"), `{
		"name": "other",
		"version": "0.1.0",
		"protocol": 1,
		"entry": "other"
	}`)

	got := ScanRoot(root)
	if len(got.Plugins) != 1 {
		t.Fatalf("plugins = %#v", got.Plugins)
	}
	if !hasDiagnostic(got.Diagnostics, "manifest_name_mismatch") {
		t.Fatalf("diagnostics = %#v", got.Diagnostics)
	}
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestLoadManifestDecodesUIAndSettingsContributions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	body := `{
		"name": "demo",
		"version": "0.1.0",
		"protocol": 1,
		"entry": "demo",
		"ui": {
			"entry": "ui/main.js",
			"components": [
				{"tag": "demo-main", "mount_id": "main", "slot": "center"}
			]
		},
		"settings": {
			"get": {"capability": "demo", "method": "config.get"},
			"save": {"capability": "demo", "method": "config.set"},
			"schema": [
				{"key": "token", "label": "Token", "type": "secret", "required": true},
				{"key": "retries", "type": "number"}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, diagnostics := LoadManifest(path)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if got.UI == nil || got.UI.Entry != "ui/main.js" || len(got.UI.Components) != 1 {
		t.Fatalf("ui = %#v", got.UI)
	}
	component := got.UI.Components[0]
	if component.Tag != "demo-main" || component.MountID != "main" || component.Slot != "center" {
		t.Fatalf("component = %#v", component)
	}
	if got.Settings == nil || got.Settings.Get.Method != "config.get" || got.Settings.Save.Method != "config.set" {
		t.Fatalf("settings = %#v", got.Settings)
	}
	if len(got.Settings.Schema) != 2 || got.Settings.Schema[0].Type != SettingsFieldSecret {
		t.Fatalf("schema = %#v", got.Settings.Schema)
	}
}

func TestLoadManifestReportsNestedUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	body := `{
		"name": "demo",
		"version": "0.1.0",
		"protocol": 1,
		"entry": "demo",
		"ui": {"entry": "ui/main.js", "pages": []},
		"settings": {
			"get": {"capability": "demo", "method": "get", "trust": true},
			"save": {"capability": "demo", "method": "set"},
			"schema": [{"key": "x", "type": "enum", "min": 0}]
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, diagnostics := LoadManifest(path)
	if got.UI == nil || got.Settings == nil {
		t.Fatalf("manifest = %#v", got)
	}
	for _, code := range []string{"ui_unknown_field", "settings_method_unknown_field", "settings_field_unknown_field", "settings_unknown_type"} {
		if !hasDiagnostic(diagnostics, code) {
			t.Fatalf("missing %s in %#v", code, diagnostics)
		}
	}
}
