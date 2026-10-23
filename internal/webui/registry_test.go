package webui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func writeUIFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("// test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testDiscovery(t *testing.T, components ...plugin.UIComponent) plugin.Discovery {
	t.Helper()
	dir := t.TempDir()
	writeUIFiles(t, dir, "ui/main.js", "ui/chat.js", "ui/theme.css", "ui/notes.txt")
	return plugin.Discovery{
		Dir: dir,
		Manifest: plugin.Manifest{
			Name:  "demo",
			Entry: "demo",
			UI: &plugin.UIManifest{
				Entry:      "ui/main.js",
				Components: components,
			},
		},
	}
}

func TestRegistryBuildsMountsAndBootstrap(t *testing.T) {
	discovery := testDiscovery(t,
		plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"},
		plugin.UIComponent{Tag: "demo-chat", MountID: "trace", Slot: "right"},
	)

	registry, diagnostics := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}

	mounts := registry.Mounts()
	if len(mounts) != 2 {
		t.Fatalf("mounts = %#v", mounts)
	}
	if mounts[0].Key != (MountKey{Plugin: "demo", ID: "chat"}) {
		t.Fatalf("mount key = %#v", mounts[0].Key)
	}
	if mounts[0].EntryURL != "/plugins/demo/ui/main.js" {
		t.Fatalf("entry URL = %q", mounts[0].EntryURL)
	}

	root, ok := registry.ResourceRoot("demo")
	expectedRoot, err := filepath.EvalSymlinks(filepath.Join(discovery.Dir, "ui"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || root != expectedRoot {
		t.Fatalf("resource root = %q, %v", root, ok)
	}
	for _, name := range []string{"ui/main.js", "ui/chat.js", "ui/theme.css", "ui/notes.txt"} {
		if _, ok := registry.ResolveResource("demo", name); !ok {
			t.Fatalf("resource %q was not resolved", name)
		}
	}
	if _, ok := registry.ResolveResource("demo", "../outside.txt"); ok {
		t.Fatal("path traversal was accepted")
	}

	bootstrap := registry.Bootstrap()
	if bootstrap.Version != 1 || len(bootstrap.Slots) != 5 || len(bootstrap.Mounts) != 2 {
		t.Fatalf("bootstrap = %#v", bootstrap)
	}
	if bootstrap.Diagnostics == nil || len(bootstrap.Diagnostics) != 0 {
		t.Fatalf("bootstrap diagnostics = %#v", bootstrap.Diagnostics)
	}
}

func TestRegistryReportsInvalidMountsAndKeepsValidOnes(t *testing.T) {
	discovery := testDiscovery(t,
		plugin.UIComponent{Tag: "other-chat", MountID: "bad-tag", Slot: "left"},
		plugin.UIComponent{Tag: "demo-chat", MountID: "Bad ID", Slot: "left"},
		plugin.UIComponent{Tag: "demo-chat", MountID: "top", Slot: "top"},
		plugin.UIComponent{Tag: "demo-chat", MountID: "valid", Slot: "left"},
		plugin.UIComponent{Tag: "demo-chat", MountID: "valid", Slot: "right"},
		plugin.UIComponent{Tag: "demo-chat", MountID: "late", Slot: "left"},
	)

	registry, diagnostics := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	// Two components may share a slot now: "valid" and "late" both target left.
	if len(registry.Mounts()) != 2 {
		t.Fatalf("mounts = %#v", registry.Mounts())
	}

	for _, code := range []string{
		"ui_tag_prefix_mismatch",
		"ui_mount_id_invalid",
		"ui_slot_reserved",
		"ui_mount_duplicate",
	} {
		if !hasCode(diagnostics, code) {
			t.Fatalf("missing %s in %#v", code, diagnostics)
		}
	}
}

func TestRegistryDiagnosesMissingAndUnsafeEntry(t *testing.T) {
	missing := testDiscovery(t, plugin.UIComponent{Tag: "demo-main", MountID: "main", Slot: "center"})
	missing.Manifest.UI.Entry = "ui/missing.js"

	unsafe := testDiscovery(t, plugin.UIComponent{Tag: "unsafe-main", MountID: "main", Slot: "center"})
	unsafe.Manifest.Name = "unsafe"
	unsafe.Manifest.UI.Entry = "../outside.js"

	registry, diagnostics := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{missing, unsafe}})
	if len(registry.Mounts()) != 0 {
		t.Fatalf("mounts = %#v", registry.Mounts())
	}
	if !hasCode(diagnostics, "ui_entry_missing") {
		t.Fatalf("missing entry diagnostic in %#v", diagnostics)
	}
	if !hasCode(diagnostics, "ui_entry_outside_package") {
		t.Fatalf("unsafe entry diagnostic in %#v", diagnostics)
	}
}

func TestBootstrapCarriesDiscoveryDiagnostics(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-main", MountID: "main", Slot: "center"})
	input := plugin.Discoveries{
		Plugins: []plugin.Discovery{discovery},
		Diagnostics: []plugin.Diagnostic{{
			Plugin: "demo", Code: "manifest_unknown_field", Message: "unknown field",
		}},
	}

	registry, _ := NewRegistry(input)
	bootstrap := registry.Bootstrap()
	if len(bootstrap.Diagnostics) != 1 || bootstrap.Diagnostics[0].Code != "manifest_unknown_field" {
		t.Fatalf("diagnostics = %#v", bootstrap.Diagnostics)
	}
}

func TestRegistryResolvesResourcesWithRelativePluginRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(cwd, "webui-relative-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	relativeDir, err := filepath.Rel(cwd, dir)
	if err != nil {
		t.Fatal(err)
	}
	writeUIFiles(t, relativeDir, "ui/main.js")

	discovery := plugin.Discovery{
		Dir: relativeDir,
		Manifest: plugin.Manifest{
			Name:  "demo",
			Entry: "demo",
			UI:    &plugin.UIManifest{Entry: "ui/main.js"},
		},
	}
	registry, diagnostics := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if _, ok := registry.ResolveResource("demo", "ui/main.js"); !ok {
		t.Fatal("resource was not resolved from a relative plugin root")
	}
}

func hasCode(diagnostics []plugin.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestInvalidEntryDoesNotReserveSlots(t *testing.T) {
	broken := testDiscovery(t, plugin.UIComponent{Tag: "demo-main", MountID: "main", Slot: "center"})
	broken.Manifest.UI.Entry = "ui/missing.js"

	working := testDiscovery(t, plugin.UIComponent{Tag: "working-main", MountID: "main", Slot: "center"})
	working.Manifest.Name = "working"
	working.Dir = filepath.Join(t.TempDir(), "working")
	writeUIFiles(t, working.Dir, "ui/main.js")
	working.Manifest.UI.Entry = "ui/main.js"

	registry, diagnostics := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{broken, working}})
	if len(registry.Mounts()) != 1 || registry.Mounts()[0].Key.Plugin != "working" {
		t.Fatalf("mounts = %#v", registry.Mounts())
	}
	if !hasCode(diagnostics, "ui_entry_missing") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestResolveResourceRejectsParentSegments(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-main", MountID: "main", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	if _, ok := registry.ResolveResource("demo", "ui/../main.js"); ok {
		t.Fatal("parent path segment was accepted")
	}
}

func TestResolveResourceRejectsPluginRootFiles(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-main", MountID: "main", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	if _, ok := registry.ResolveResource("demo", "plugin.json"); ok {
		t.Fatal("plugin root file was served outside the UI resource root")
	}
}
