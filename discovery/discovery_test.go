package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanValidAndInvalid(t *testing.T) {
	root := t.TempDir()

	// valid echo plugin
	writeFile(t, filepath.Join(root, "echo", "plugin.json"), `{
		"name": "echo",
		"version": "0.1.0",
		"protocol": 1,
		"provides": ["echo"],
		"consumes": [],
		"entry": "echo.exe"
	}`)
	writeFile(t, filepath.Join(root, "echo", "echo.exe"), "bin")

	// invalid: missing name
	writeFile(t, filepath.Join(root, "bad-name", "plugin.json"), `{
		"version": "0.1.0",
		"protocol": 1,
		"provides": [],
		"entry": "x"
	}`)
	writeFile(t, filepath.Join(root, "bad-name", "x"), "bin")

	// invalid: entry file missing
	writeFile(t, filepath.Join(root, "no-entry", "plugin.json"), `{
		"name": "no-entry",
		"version": "0.1.0",
		"protocol": 1,
		"provides": ["x"],
		"entry": "missing.exe"
	}`)

	// not a plugin dir (no plugin.json) — ignored
	writeFile(t, filepath.Join(root, "notes", "readme.txt"), "hi")

	res := Scan(root)
	if len(res.Plugins) != 1 {
		t.Fatalf("want 1 plugin, got %d: %+v", len(res.Plugins), res.Plugins)
	}
	if res.Plugins[0].Manifest.Name != "echo" {
		t.Fatalf("want echo, got %s", res.Plugins[0].Manifest.Name)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("want 2 errors, got %d: %v", len(res.Errors), res.Errors)
	}
}

func TestScanMissingRoot(t *testing.T) {
	res := Scan(filepath.Join(t.TempDir(), "nope"))
	if len(res.Errors) != 1 || len(res.Plugins) != 0 {
		t.Fatalf("want 1 root error, got plugins=%d errors=%v", len(res.Plugins), res.Errors)
	}
}
