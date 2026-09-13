package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileToolsIntegration mounts filetools alongside session+fakellm and runs a turn.
// fakellm calls the first tool (read_file) with {"text":"..."} which fails because
// path is required — this verifies the full call path: Host → tools.call → filetools
// error → tool_result in Session Log → final assistant reply.
func TestFileToolsIntegration(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildFileToolsPluginDir(t, root, pluginsDir, "filetools")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","filetools"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "read something",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("filetools turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	// fakellm calls read_file which fails with "path is required"
	if !strings.Contains(s, "tool_call") {
		t.Fatalf("want tool_call observed in turn: %s", s)
	}
	if !strings.Contains(s, "read_file") {
		t.Fatalf("want read_file tool name in output: %s", s)
	}
	// Tool error should be logged and turn should still complete.
	if !strings.Contains(s, "Tool said:") {
		t.Fatalf("want assistant reply after tool result: %s", s)
	}
	if !strings.Contains(s, "derive ok") {
		t.Fatalf("want derive after turn: %s", s)
	}
	if !strings.Contains(s, `"role":"tool"`) {
		t.Fatalf("want tool role in derived context: %s", s)
	}
}
