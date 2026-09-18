package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
)

func TestPluginSwitchLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := SwitchPath(dir)
	if got := LoadPluginSwitchFile(path); len(got) != 0 {
		t.Fatalf("expected empty disabled set, got %v", got)
	}
	set := map[string]bool{"sandbox": true, "filetools": true}
	if err := savePluginSwitchFile(path, set); err != nil {
		t.Fatal(err)
	}
	got := LoadPluginSwitchFile(path)
	if !got["sandbox"] || !got["filetools"] || len(got) != 2 {
		t.Fatalf("reload mismatch: %v", got)
	}
	raw, _ := os.ReadFile(path)
	var f pluginSwitchFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Disabled) != 2 || f.Disabled[0] != "filetools" {
		t.Fatalf("persist shape: %v", f.Disabled)
	}
}

func TestFilterMountedFound(t *testing.T) {
	mounted := []discovery.Found{
		{Manifest: plugin.Manifest{Name: "agent"}},
		{Manifest: plugin.Manifest{Name: "sandbox"}},
	}
	out := FilterMountedFound(mounted, map[string]bool{"sandbox": true})
	if len(out) != 1 || out[0].Manifest.Name != "agent" {
		t.Fatalf("filter result: %+v", out)
	}
	if len(FilterMountedFound(mounted, nil)) != 2 {
		t.Fatal("nil disabled should keep all")
	}
}

func TestSetPluginEnabledPersistsAndUnlists(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		plugins:     map[string]*proc{},
		mountedUI:   map[string]bool{},
		disabled:    map[string]bool{},
		pluginsDir:  dir,
		switchPath:  SwitchPath(dir),
		provides:    map[string]string{},
		degraded:    map[string]bool{},
		pending:     map[string]*wait{},
		gen:         map[string]int{},
		approvals:   nil,
	}
	out, err := s.SetPluginEnabled("demo", false)
	if err != nil {
		t.Fatal(err)
	}
	if out["enabled"] != false {
		t.Fatalf("out: %v", out)
	}
	if !s.IsPluginDisabled("demo") {
		t.Fatal("expected disabled in memory")
	}
	if !LoadPluginSwitchFile(filepath.Join(dir, SwitchFileName))["demo"] {
		t.Fatal("expected disabled on disk")
	}
	if _, err := s.SetPluginEnabled("demo", true); err != nil {
		t.Fatal(err)
	}
	if s.IsPluginDisabled("demo") {
		t.Fatal("expected re-enabled")
	}
	if LoadPluginSwitchFile(filepath.Join(dir, SwitchFileName))["demo"] {
		t.Fatal("expected removed from disk")
	}
}

func TestEnsurePluginsSkipsDisabled(t *testing.T) {
	dir := t.TempDir()
	// UI-only plugin: empty entry + ui entry file must exist for Discovery.
	pdir := filepath.Join(dir, "echo-ui")
	if err := os.MkdirAll(filepath.Join(pdir, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	pj := `{"name":"echo-ui","version":"0.1.0","protocol":5,"provides":[],"entry":"","ui":{"entry":"main.js"}}`
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(pj), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "ui", "main.js"), []byte("/* test */"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		plugins:    map[string]*proc{},
		mountedUI:  map[string]bool{},
		disabled:   map[string]bool{"echo-ui": true},
		pluginsDir: dir,
		switchPath: SwitchPath(dir),
		provides:   map[string]string{},
		degraded:   map[string]bool{},
		pending:    map[string]*wait{},
		gen:        map[string]int{},
	}
	res, err := s.EnsurePlugins([]string{"echo-ui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Disabled) != 1 || res.Disabled[0] != "echo-ui" {
		t.Fatalf("ensure result: %+v missing=%v", res, res.Missing)
	}
	if s.mountedUI["echo-ui"] {
		t.Fatal("disabled plugin must not mount UI")
	}
}

func TestCallHostSetPluginEnabled(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		plugins:    map[string]*proc{},
		mountedUI:  map[string]bool{},
		disabled:   map[string]bool{},
		pluginsDir: dir,
		switchPath: SwitchPath(dir),
		provides:   map[string]string{},
		degraded:   map[string]bool{},
		pending:    map[string]*wait{},
		gen:        map[string]int{},
	}
	raw, err := CallHost(s, "setPluginEnabled", json.RawMessage(`{"name":"x","enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["ok"] != true || out["enabled"] != false {
		t.Fatalf("call host out: %v raw=%s", out, raw)
	}
	if _, err := CallHost(s, "nope", nil); err == nil {
		t.Fatal("expected method_not_found")
	}
}
