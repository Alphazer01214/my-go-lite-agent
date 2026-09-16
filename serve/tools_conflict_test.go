package serve_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/serve"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func buildBin(t *testing.T, root, pkg, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, b)
	}
}

func installToolsPlugin(t *testing.T, pluginsDir, name, bin string) {
	t.Helper()
	dir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := name
	if runtime.GOOS == "windows" {
		exe = name + ".exe"
	}
	dst := filepath.Join(dir, exe)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"` + name + `","version":"0.1.0","protocol":3,"autostart":true,"provides":["tools"],"consumes":[],"entry":"` + exe + `"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateToolNameFailsMount(t *testing.T) {
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "echotool")
	buildBin(t, root, "./plugins/echotool", bin)

	pluginsDir := t.TempDir()
	installToolsPlugin(t, pluginsDir, "echo-a", bin)
	installToolsPlugin(t, pluginsDir, "echo-b", bin)

	res := discovery.Scan(pluginsDir)
	plan := assembly.Resolve(assembly.Config{Plugins: []string{"echo-a", "echo-b"}}, res)
	if len(plan.Mounted) != 2 {
		t.Fatalf("want 2 mounted, got %+v", plan)
	}
	_, err := serve.Start(plan.Mounted)
	if err == nil {
		t.Fatal("want fail-loud on duplicate tool name")
	}
	if !strings.Contains(err.Error(), "provided by both") && !strings.Contains(err.Error(), "tool") {
		t.Fatalf("want tool conflict error, got: %v", err)
	}
}

func TestMultiToolsProvidersStart(t *testing.T) {
	root := moduleRoot(t)
	echoBin := filepath.Join(t.TempDir(), "echotool")
	emptyBin := filepath.Join(t.TempDir(), "emptytools")
	buildBin(t, root, "./plugins/echotool", echoBin)
	buildBin(t, root, "./plugins/emptytools", emptyBin)

	pluginsDir := t.TempDir()
	installToolsPlugin(t, pluginsDir, "echotool", echoBin)
	installToolsPlugin(t, pluginsDir, "emptytools", emptyBin)

	res := discovery.Scan(pluginsDir)
	plan := assembly.Resolve(assembly.Config{Plugins: []string{"echotool", "emptytools"}}, res)
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		t.Fatalf("multi tools start: %v", err)
	}
	defer func() { _ = srv.Close() }()
	payload, err := srv.CallByCap("tools", "list", nil)
	if err != nil {
		t.Fatalf("tools.list: %v", err)
	}
	if !strings.Contains(string(payload), "name") {
		t.Fatalf("want merged tools list: %s", payload)
	}
}
