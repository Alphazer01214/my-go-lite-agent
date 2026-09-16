package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// UI-only Plugins (ADR-0011): manifest + ui/ assets, no executable, no
// process, no Frames — yet Discovery sees them and Assembly mounts them.
func TestUIOnlyPluginMountsWithoutProcess(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	writeFile(t, filepath.Join(pluginsDir, "uifix", "plugin.json"), `{
		"name": "uifix",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": [],
		"consumes": [],
		"ui": {"entry": "main.js", "mounts": [{"slot": "sidebar", "component": "uifix-panel", "props": {"n": 1}}]}
	}`)
	writeFile(t, filepath.Join(pluginsDir, "uifix", "ui", "main.js"), "export{};")

	// Discovery sees it without an executable.
	out, err := exec.Command(hostBin, "-discover", pluginsDir).CombinedOutput()
	if err != nil {
		t.Fatalf("discover ui-only: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "uifix") {
		t.Fatalf("want uifix discovered: %s", out)
	}

	// Assembly mounts it; the exe probe is skipped.
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["uifix"]}`)
	out, err = exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-dump").CombinedOutput()
	if err != nil {
		t.Fatalf("mount ui-only: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "mount name=uifix") {
		t.Fatalf("want mount line: %s", out)
	}

	// serve.Start accepts the plan; the plugin owns no capability and
	// answers no Frames (it has no process).
	out, err = exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-call-plugin", "uifix").CombinedOutput()
	if err == nil {
		t.Fatalf("ui-only must not answer Frames: %s", out)
	}
	if !strings.Contains(string(out), "uifix") {
		t.Fatalf("want failure naming the plugin: %s", out)
	}
}

func TestManifestWithoutEntryOrUIRejected(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	writeFile(t, filepath.Join(pluginsDir, "hollow", "plugin.json"), `{
		"name": "hollow",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": []
	}`)

	out, err := exec.Command(hostBin, "-discover", pluginsDir).CombinedOutput()
	if err == nil {
		t.Fatalf("manifest with neither entry nor ui must fail discovery: %s", out)
	}
	if !strings.Contains(string(out), "entry or ui is required") {
		t.Fatalf("want entry-or-ui diagnosis: %s", out)
	}
}
