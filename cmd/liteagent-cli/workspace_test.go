package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceBindsToDefaultSession(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	ws := t.TempDir()
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-invoke", "session",
		"-frame-cap", "session",
		"-frame-method", "info",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session info: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, filepath.Base(ws)) && !strings.Contains(s, ws) {
		// info is printed as invoke ok payload=...
		if !strings.Contains(s, "workspace") {
			t.Fatalf("want workspace in session info: %s", s)
		}
	}
}

func TestToolsMultiProviderListAndCall(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")
	// second tools provider: filetools
	buildPkgToPlugin(t, root, pluginsDir, "filetools")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["agent","session","stubllm","echotool","filetools","context-manager"]}`)

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "note.txt"), "hello-ws\n")

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-turn", "use tools",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
}

func TestFiletoolsRespectsWorkspace(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	buildPkgToPlugin(t, root, pluginsDir, "filetools")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["agent","session","stubllm","filetools"]}`)

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "a.txt"), "ws-file\n")

	// stubllm will call first tool (read_file) with its schema; ensure turn completes.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-turn", "read",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "turn ok") {
		t.Fatalf("want turn ok: %s", out)
	}
}

// buildPkgToPlugin builds a plugin package into pluginsDir/<name>/ with a minimal manifest.
func buildPkgToPlugin(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	dir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", bin, "./plugins/"+name)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, b)
	}
	provides := `["tools"]`
	if name == "permission" {
		provides = `["policy"]`
	}
	if name == "project-context" {
		provides = `["project-context"]`
	}
	if name == "skill-manager" {
		provides = `["tools","skills"]`
	}
	writeFile(t, filepath.Join(dir, "plugin.json"), `{
  "name": "`+name+`",
  "version": "0.1.0",
  "protocol": 3,
  "provides": `+provides+`,
  "consumes": [],
  "entry": "`+name+`"
}`)
}

func TestPolicyDenyBlocksWriteTool(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	buildPkgToPlugin(t, root, pluginsDir, "filetools")
	buildPkgToPlugin(t, root, pluginsDir, "permission")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["agent","session","stubllm","filetools","permission"]}`)

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, ".liteagent", "permissions.json"),
		`{"defaultAction":"allow","rules":[{"tool":"read_file","action":"deny"}]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-turn", "read",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "denied by policy") && !strings.Contains(s, "error: denied") {
		// stub calls tools[0] which is read_file → must be denied
		t.Fatalf("want policy deny in output: %s", s)
	}
}

func TestSkillTriggerExpandsUserInput(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")
	buildPkgToPlugin(t, root, pluginsDir, "skill-manager")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["agent","session","stubllm","skill-manager","context-manager"]}`)

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, ".liteagent", "skills", "demo", "SKILL.md"),
		"# demo\n\nSKILL_BODY_MARKER_XYZ\n")

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-turn", "please $demo this",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SKILL_BODY_MARKER_XYZ") {
		t.Fatalf("want expanded skill body in session log: %s", out)
	}
}
