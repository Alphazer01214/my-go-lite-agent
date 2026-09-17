package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	ok := Manifest{Name: "a", Version: "1.0.0", Protocol: CurrentProtocol, Entry: "a"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	// UI-only Plugin: legal with no executable entry (ADR-0011).
	uiOnly := Manifest{Name: "a", Version: "1.0.0", Protocol: CurrentProtocol,
		UI: &UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "sidebar", Component: "a-panel"}}}}
	if err := uiOnly.Validate(); err != nil {
		t.Fatalf("ui-only manifest rejected: %v", err)
	}

	cases := []struct {
		name string
		m    Manifest
	}{
		{"missing name", Manifest{Version: "1", Protocol: 2, Entry: "x"}},
		{"missing version", Manifest{Name: "a", Protocol: 2, Entry: "x"}},
		{"bad protocol", Manifest{Name: "a", Version: "1", Protocol: 6, Entry: "x"}},
		{"missing entry and ui", Manifest{Name: "a", Version: "1", Protocol: 2}},
		{"empty provides item", Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "x", Provides: []string{""}}},
		{"bad name chars", Manifest{Name: "Help", Version: "1", Protocol: 2, Entry: "x"}},
		{"bad hostFace", Manifest{Name: "a", Version: "1", Protocol: 4, Entry: "x", HostFaces: []string{"wat"}}},
		{"duplicate hostFace", Manifest{Name: "a", Version: "1", Protocol: 4, Entry: "x", HostFaces: []string{"config", "config"}}},
	}
	for _, tc := range cases {
		if err := tc.m.Validate(); err == nil {
			t.Errorf("%s: want error", tc.name)
		}
	}
}

func TestLoadManifestStripsUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	body := `{"name":"echo","version":"1.0.0","protocol":5,"entry":"echo.exe"}`
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, body...), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("BOM-prefixed manifest rejected: %v", err)
	}
	if m.Name != "echo" || m.Entry != "echo.exe" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestEntryExists(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "bin"}
	if err := m.EntryExists(dir); err == nil {
		t.Fatal("want missing entry error")
	}
	if err := os.WriteFile(filepath.Join(dir, "bin"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.EntryExists(dir); err != nil {
		t.Fatalf("entry should exist: %v", err)
	}
}

func TestConflictsWithNativeCommand(t *testing.T) {
	if !(Manifest{Name: "help"}).ConflictsWithNativeCommand() {
		t.Fatal("help must conflict")
	}
	if !(Manifest{Name: "LP"}).ConflictsWithNativeCommand() {
		t.Fatal("LP must conflict case-insensitively")
	}
	if (Manifest{Name: "llm-openai"}).ConflictsWithNativeCommand() {
		t.Fatal("llm-openai must not conflict")
	}
}

func TestManifestCommandsValidate(t *testing.T) {
	ok := Manifest{
		Name: "llm-openai", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		Commands: []CommandSpec{{Name: "config", Description: "cfg", Usage: "/llm-openai config"}},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("commands rejected: %v", err)
	}
	bad := Manifest{
		Name: "p", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		Commands: []CommandSpec{{Name: "has space"}},
	}
	if err := bad.Validate(); err == nil {
		t.Fatal("want whitespace command name error")
	}
}

func TestUISpecValidate(t *testing.T) {
	ok := Manifest{
		Name: "uidemo", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		UI: &UISpec{
			Entry: "main.js",
			Mounts: []UIMount{
				{Slot: "sidebar", Component: "uidemo-mode-panel", Props: json.RawMessage(`{"mode":"chat"}`)},
			},
		},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid ui spec rejected: %v", err)
	}

	// entry may live nested under ui/.
	nested := Manifest{Name: "uidemo", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		UI: &UISpec{Entry: "ui/nested/main.js"}}
	if err := nested.Validate(); err != nil {
		t.Fatalf("nested entry rejected: %v", err)
	}

	// mounts are optional (dynamic PanelOp-only plugins).
	dynamic := Manifest{Name: "uidemo", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		UI: &UISpec{Entry: "main.js"}}
	if err := dynamic.Validate(); err != nil {
		t.Fatalf("mount-less ui spec rejected: %v", err)
	}

	// declared assets are legal (ADR-0011 multi-file contract).
	withAssets := Manifest{Name: "uidemo", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		UI: &UISpec{Entry: "main.js", Assets: []string{"panels.css", "mode-panel.html"}}}
	if err := withAssets.Validate(); err != nil {
		t.Fatalf("asset-declaring ui spec rejected: %v", err)
	}

	// page dimension (ADR-0011): syntax-validated here, vocabulary lives in the layout.
	paged := Manifest{Name: "uidemo", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		UI: &UISpec{Entry: "main.js", Mounts: []UIMount{{Page: "trace", Slot: "main", Component: "uidemo-x"}}}}
	if err := paged.Validate(); err != nil {
		t.Fatalf("page-qualified mount rejected: %v", err)
	}

	cases := []struct {
		name string
		ui   UISpec
	}{
		{"missing entry", UISpec{Mounts: nil}},
		{"entry not js", UISpec{Entry: "index.html"}},
		{"entry escapes ui", UISpec{Entry: "../evil/main.js"}},
		{"entry absolute", UISpec{Entry: "/etc/main.js"}},
		{"asset escapes ui", UISpec{Entry: "main.js", Assets: []string{"../evil/panels.css"}}},
		{"asset absolute", UISpec{Entry: "main.js", Assets: []string{"/etc/panels.css"}}},
		{"asset empty", UISpec{Entry: "main.js", Assets: []string{"  "}}},
		{"asset duplicates entry", UISpec{Entry: "main.js", Assets: []string{"main.js"}}},
		{"page uppercase", UISpec{Entry: "main.js", Mounts: []UIMount{{Page: "Main", Slot: "main", Component: "uidemo-x"}}}},
		{"page with space", UISpec{Entry: "main.js", Mounts: []UIMount{{Page: "main page", Slot: "main", Component: "uidemo-x"}}}},
		{"bad slot", UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "Footer", Component: "uidemo-x"}}}},
		{"foreign component prefix", UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "sidebar", Component: "other-panel"}}}},
		{"component without hyphen", UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "sidebar", Component: "uidemo"}}}},
		{"component bad chars", UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "sidebar", Component: "uidemo-X"}}}},
		{"props not object", UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "sidebar", Component: "uidemo-x", Props: json.RawMessage(`[1]`)}}}},
		{"props malformed", UISpec{Entry: "main.js", Mounts: []UIMount{{Slot: "sidebar", Component: "uidemo-x", Props: json.RawMessage(`{`)}}}},
	}
	for _, tc := range cases {
		m := Manifest{Name: "uidemo", Version: "1", Protocol: CurrentProtocol, Entry: "x", UI: &tc.ui}
		if err := m.Validate(); err == nil {
			t.Errorf("%s: want error", tc.name)
		}
	}
}

func TestUISpecNormalizedEntry(t *testing.T) {
	for in, want := range map[string]string{
		"main.js":        "ui/main.js",
		"ui/main.js":     "ui/main.js",
		"ui/nested/a.js": "ui/nested/a.js",
		`sub\win.js`:     "ui/sub/win.js",
	} {
		if got := (UISpec{Entry: in}).NormalizedEntry(); got != want {
			t.Errorf("NormalizedEntry(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUIEntryExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "bin",
		UI: &UISpec{Entry: "main.js"}}
	if err := m.UIEntryExists(dir); err == nil {
		t.Fatal("want missing ui.entry error")
	}
	if err := os.WriteFile(filepath.Join(dir, "ui", "main.js"), []byte("export{};"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.UIEntryExists(dir); err != nil {
		t.Fatalf("ui.entry should exist: %v", err)
	}
}

func TestUIAssetsExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "bin",
		UI: &UISpec{Entry: "main.js", Assets: []string{"panels.css", "tpl.html"}}}
	if err := m.UIAssetsExist(dir); err == nil {
		t.Fatal("want missing ui.asset error")
	}
	if err := os.WriteFile(filepath.Join(dir, "ui", "panels.css"), []byte(":host{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ui", "tpl.html"), []byte("<template></template>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.UIAssetsExist(dir); err != nil {
		t.Fatalf("assets should exist: %v", err)
	}
	// No assets declared → vacuously fine.
	none := Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "bin", UI: &UISpec{Entry: "main.js"}}
	if err := none.UIAssetsExist(dir); err != nil {
		t.Fatalf("asset-less spec must pass: %v", err)
	}
}

func TestValidUISlot(t *testing.T) {
	for _, s := range UISlots {
		if !ValidUISlot(s) {
			t.Errorf("declared slot %q must be valid", s)
		}
	}
	if ValidUISlot("footer") {
		t.Error("footer must not be a slot")
	}
}
