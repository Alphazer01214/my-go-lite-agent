package web_test

import (
	"encoding/json"
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
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func buildNamedPlugin(t *testing.T, root, pkg, pluginsDir, name, provides, faces string) {
	t.Helper()
	dir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := name
	if runtime.GOOS == "windows" {
		exe = name + ".exe"
	}
	bin := filepath.Join(dir, exe)
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, b)
	}
	manifest := `{"name":"` + name + `","version":"0.1.0","protocol":4,"autostart":true,"provides":` + provides + `,"hostFaces":` + faces + `,"consumes":[],"entry":"` + exe + `"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCallByFaceConfigFace(t *testing.T) {
	root := moduleRoot(t)
	pluginsDir := t.TempDir()
	buildNamedPlugin(t, root, "./plugins/session", pluginsDir, "session", `["session"]`, `["config","commands"]`)
	buildNamedPlugin(t, root, "./plugins/llm-openai", pluginsDir, "llm-openai", `["llm"]`, `["config","commands"]`)

	res := discovery.Scan(pluginsDir)
	plan := assembly.Resolve(assembly.Config{Plugins: []string{"session", "llm-openai"}}, res)
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Close() }()

	schema, err := serve.CallByFace(srv, "llm-openai", "config", "schema", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	if !strings.Contains(string(schema), "apiKey") {
		t.Fatalf("want apiKey field: %s", schema)
	}
	get, err := serve.CallByFace(srv, "llm-openai", "config", "get", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(string(get), "fields") {
		t.Fatalf("want fields: %s", get)
	}
	// session also has a config face — per-plugin routing must not collide.
	sess, err := serve.CallByFace(srv, "session", "config", "schema", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("session schema: %v", err)
	}
	if !strings.Contains(string(sess), "fullToolResults") {
		t.Fatalf("want session config schema: %s", sess)
	}
}
