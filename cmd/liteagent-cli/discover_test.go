package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverListsPlugins(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "echo", "plugin.json"), `{
		"name": "echo",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["echo"],
		"consumes": [],
		"entry": "echo.exe"
	}`)
	writeFile(t, filepath.Join(dir, "echo", "echo.exe"), "bin")

	cmd := exec.Command(hostBin, "-discover", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("discover: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "echo") || !strings.Contains(s, "provides=[echo]") {
		t.Fatalf("unexpected discover output: %s", s)
	}
	if strings.Contains(s, "ok id=") {
		t.Fatalf("discovery must not start plugins: %s", s)
	}
}

func TestDiscoverInvalidManifestFails(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad", "plugin.json"), `{"version":"1","protocol": 2,
		"autostart": true,"entry":"x"}`)
	writeFile(t, filepath.Join(dir, "bad", "x"), "bin")

	cmd := exec.Command(hostBin, "-discover", dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want non-zero exit, got: %s", out)
	}
	s := string(out)
	if !strings.Contains(s, "discover error") {
		t.Fatalf("want discover error, got: %s", s)
	}
	if !strings.Contains(s, "name") {
		t.Fatalf("want field-level diagnosis, got: %s", s)
	}
}
