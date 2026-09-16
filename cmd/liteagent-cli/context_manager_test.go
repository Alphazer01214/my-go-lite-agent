package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildContextManagerPluginDir(t *testing.T, root, pluginsDir, name string, segmentsJSON string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/context-manager")
	dir := filepath.Join(pluginsDir, name)
	dst := filepath.Join(dir, name+".exe")
	writeFile(t, filepath.Join(dir, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["system-prompt", "context"],
		"consumes": [],
		"entry": "`+name+`.exe",
		"commands": [
			{"name":"usage","description":"Last prepare Context Usage","usage":"/`+name+` usage [sessionId]"},
			{"name":"list","description":"Model Context messages from last prepare","usage":"/`+name+` list [sessionId]"},
			{"name":"skills","description":"List registered skills","usage":"/`+name+` skills"}
		]
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
	if segmentsJSON != "" {
		writeFile(t, filepath.Join(dir, "segments.json"), segmentsJSON)
	}
}

// TestContextManagerAssemblesSystemPrompt is the main-seam test for ticket 01:
// Context Manager plugin provides system-prompt; Agent Loop appends assembled
// System Prompt as a Session Log fact; derive includes it in Model Context.
func TestContextManagerAssemblesSystemPrompt(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [
			{"name": "identity", "order": -1000, "text": "You are a lite agent."},
			{"name": "rules", "order": 100, "text": "Be brief."}
		]
	}`)
	buildAgentPluginDir(t, root, pluginsDir, "agent")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","context-manager","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello context",
		"-session-derive",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn with context manager: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	// Assembled System Prompt must appear in derived Model Context.
	if !strings.Contains(s, "You are a lite agent.") {
		t.Fatalf("want identity segment in derive: %s", s)
	}
	if !strings.Contains(s, "Be brief.") {
		t.Fatalf("want rules segment in derive: %s", s)
	}
	if !strings.Contains(s, `"role":"system"`) && !strings.Contains(s, `"role": "system"`) {
		t.Fatalf("want system role in derive: %s", s)
	}
	// Session Log must contain the system fact (query).
	if !strings.Contains(s, "You are a lite agent.") {
		t.Fatalf("want system prompt in session query: %s", s)
	}
}

func buildPromptRegPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/promptreg")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["demo"],
		"consumes": ["system-prompt"],
		"entry": "`+name+`.exe"
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestContextManagerRegisterSegmentViaStar: another Plugin registers a dynamic Prompt Segment.
func TestContextManagerRegisterSegmentViaStar(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")
	buildPromptRegPluginDir(t, root, pluginsDir, "promptreg")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","context-manager","promptreg","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "promptreg",
		"-invoke-payload", `{"name":"dyn","order":50,"text":"DYNAMIC_SEGMENT_MARKER"}`,
		"-turn", "hello dynamic",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("register segment then turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, "DYNAMIC_SEGMENT_MARKER") {
		t.Fatalf("want dynamically registered segment in derive: %s", s)
	}
}

// TestTurnWorksWithoutContextManager: missing system-prompt provider must not fail the turn.
func TestTurnWorksWithoutContextManager(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn without context manager: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "turn ok") {
		t.Fatalf("want turn ok without system-prompt provider: %s", out)
	}
}
