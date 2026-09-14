package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoopSkipsDuplicateSystemAppend: second turn with unchanged System Prompt
// must not stack another system fact into Model Context.
func TestLoopSkipsDuplicateSystemAppend(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [{"name":"identity","order":-1000,"text":"CM_STABLE_SYSTEM"}]
	}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","context-manager"]}`)
	env := hostEnv(t)

	cmd1 := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-turn", "first")
	cmd1.Env = env
	if out, err := cmd1.CombinedOutput(); err != nil {
		t.Fatalf("turn1: %v\n%s", err, out)
	}

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "second",
		"-session-derive",
	)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn2: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "CM_STABLE_SYSTEM") {
		t.Fatalf("want stable system in derive: %s", s)
	}
	// Count system projections in derive line.
	var deriveLine string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "derive ok messages=") {
			deriveLine = strings.TrimSpace(line)
			break
		}
	}
	if deriveLine == "" {
		t.Fatalf("want derive: %s", s)
	}
	if n := strings.Count(deriveLine, `"role":"system"`); n != 1 {
		t.Fatalf("want exactly one system in derive, got %d: %s", n, deriveLine)
	}
}

// TestLoopAutoCompactsWhenOverBudget: fat prior history + CM must compact and still complete.
func TestLoopAutoCompactsWhenOverBudget(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [{"name":"identity","order":-1000,"text":"CM_COMPACT_SYSTEM"}]
	}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","context-manager"]}`)

	fat := strings.Repeat("x", 8000)
	appendJSON := `[{"type":"message","role":"user","content":"` + fat + `"},{"type":"message","role":"assistant","content":"` + fat + `"},{"type":"message","role":"user","content":"` + fat + `"},{"type":"message","role":"assistant","content":"` + fat + `"}]`

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", appendJSON,
		"-turn", "compact me",
		"-session-derive",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fat history turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok after compact: %s", s)
	}
	if !strings.Contains(s, "Conversation summary") && !strings.Contains(s, "context_summary") {
		t.Fatalf("want context_summary in log: %s", s)
	}
	if !strings.Contains(s, "CM_COMPACT_SYSTEM") {
		t.Fatalf("want system re-appended after compact: %s", s)
	}
}

// TestCLIContextUsageAndList: turn prints context usage; -context-list lists prepare messages.
func TestCLIContextUsageAndList(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [{"name":"identity","order":-1000,"text":"CLI_USAGE_SYSTEM"}]
	}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","context-manager"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello usage",
		"-context-list", "5",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn+list: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if !strings.Contains(s, "context usage=") {
		t.Fatalf("want context usage line: %s", s)
	}
	if !strings.Contains(s, "context list count=") {
		t.Fatalf("want context list line: %s", s)
	}
	if !strings.Contains(s, "hello usage") {
		t.Fatalf("list should include user message: %s", s)
	}
}
