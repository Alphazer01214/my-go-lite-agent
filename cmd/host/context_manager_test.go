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
	bin := buildPkg(t, root, "./plugins/contextmanager")
	dir := filepath.Join(pluginsDir, name)
	dst := filepath.Join(dir, name+".exe")
	writeFile(t, filepath.Join(dir, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 1,
		"provides": ["system-prompt"],
		"consumes": [],
		"entry": "`+name+`.exe"
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
// Context Manager plugin provides system-prompt; default Loop appends assembled
// System Prompt as a Session Log fact; derive includes it in Model Context.
func TestContextManagerAssemblesSystemPrompt(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "contextmanager", `{
		"segments": [
			{"name": "identity", "order": -1000, "text": "You are a lite agent."},
			{"name": "rules", "order": 100, "text": "Be brief."}
		]
	}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","contextmanager"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello context",
		"-session-derive",
		"-session-query",
	)
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
		"protocol": 1,
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
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "contextmanager", "")
	buildPromptRegPluginDir(t, root, pluginsDir, "promptreg")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","contextmanager","promptreg"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "promptreg",
		"-invoke-payload", `{"name":"dyn","order":50,"text":"DYNAMIC_SEGMENT_MARKER"}`,
		"-turn", "hello dynamic",
		"-session-derive",
	)
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
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn without context manager: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "turn ok") {
		t.Fatalf("want turn ok without system-prompt provider: %s", out)
	}
}
