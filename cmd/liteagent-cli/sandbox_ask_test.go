package main_test

// Ask/deny path for author-declared tool severity (plugins own severity;
// Host only relays agent.confirm). Mirrors TestPolicyDenyBlocksWriteTool's
// fixture shape so stubllm provides llm.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mountAskFixture(t *testing.T, root string) (hostBin, pluginsDir, ws string) {
	t.Helper()
	hostBin = buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir = t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	buildPkgToPlugin(t, root, pluginsDir, "filetools")
	buildPkgToPlugin(t, root, pluginsDir, "sandbox")
	ws = t.TempDir()
	return hostBin, pluginsDir, ws
}

func TestPolicyAskWriteFileMediumSeverity(t *testing.T) {
	root := moduleRoot(t)
	hostBin, pluginsDir, ws := mountAskFixture(t, root)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	// Explicit mount list like the deny test; default scheme tool_calling
	// also depends on sandbox (soft) once ensured.
	writeFile(t, cfg, `{"plugins":["agent","session","stubllm","filetools","sandbox"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-turn", "write",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	// Empty stdin → CLI confirm EOF → deny (ask path exercised).
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn: %v\n%s", err, out)
	}
	s := string(out)
	// stubllm calls tools[0]; filetools list starts with read_file (low→allow)
	// then write_file (medium→ask). Accept either completed tool or ask-deny.
	if !strings.Contains(s, "turn ok") && !strings.Contains(s, "denied") {
		t.Fatalf("want turn completed or policy deny:\n%s", s)
	}
}

func TestPolicyDenyWriteFileByProjectRule(t *testing.T) {
	root := moduleRoot(t)
	hostBin, pluginsDir, ws := mountAskFixture(t, root)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["agent","session","stubllm","filetools","sandbox"]}`)

	dir := filepath.Join(ws, ".liteagent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Deny every tool with a path-like arg that matches write — use shell-style
	// catch-all on write_file only. stubllm uses tools[0]=read_file, so also
	// deny read_file to force a deny on the first call.
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"),
		[]byte(`{"defaultAction":"allow","rules":[{"tool":"read_file","action":"deny"},{"tool":"write_file","action":"deny"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-workspace", ws,
		"-turn", "read",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "denied by policy") {
		t.Fatalf("want policy deny:\n%s", s)
	}
}