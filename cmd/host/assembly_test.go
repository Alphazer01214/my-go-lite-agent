package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildEchoPluginDir creates dir/<name>/ with a working echo plugin binary.
func buildEchoPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	echoBin := buildPkg(t, root, "./plugins/echo")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 1,
		"provides": ["echo"],
		"consumes": [],
		"entry": "`+name+`.exe"
	}`)
	copyFile(t, echoBin, dst)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestAssemblyMountsOnlyNamedPlugins(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "alpha")
	buildEchoPluginDir(t, root, pluginsDir, "beta")

	// Config A: only alpha
	cfgA := filepath.Join(t.TempDir(), "a.json")
	writeFile(t, cfgA, `{"plugins":["alpha"]}`)
	outA, err := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfgA, "-dump").CombinedOutput()
	if err != nil {
		t.Fatalf("assembly A: %v\n%s", err, outA)
	}
	sA := string(outA)
	if !strings.Contains(sA, "mount name=alpha") || !strings.Contains(sA, "ok id=alpha") {
		t.Fatalf("A should mount alpha: %s", sA)
	}
	if strings.Contains(sA, "mount name=beta") || strings.Contains(sA, "ok id=beta") {
		t.Fatalf("A must not start beta: %s", sA)
	}
	if !strings.Contains(sA, "available name=beta mounted=false") {
		t.Fatalf("A dump should list beta as available: %s", sA)
	}

	// Config B: both
	cfgB := filepath.Join(t.TempDir(), "b.json")
	writeFile(t, cfgB, `{"plugins":["alpha","beta"]}`)
	outB, err := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfgB, "-dump").CombinedOutput()
	if err != nil {
		t.Fatalf("assembly B: %v\n%s", err, outB)
	}
	sB := string(outB)
	if !strings.Contains(sB, "ok id=alpha") || !strings.Contains(sB, "ok id=beta") {
		t.Fatalf("B should mount both: %s", sB)
	}
}

func TestAssemblyMissingPluginFails(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "alpha")

	cfg := filepath.Join(t.TempDir(), "bad.json")
	writeFile(t, cfg, `{"plugins":["alpha","ghost"]}`)
	out, err := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg).CombinedOutput()
	if err == nil {
		t.Fatalf("want fail-loud, got: %s", out)
	}
	if !strings.Contains(string(out), "ghost") {
		t.Fatalf("want missing plugin name in error: %s", out)
	}
}
