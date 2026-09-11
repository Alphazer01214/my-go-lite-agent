package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildEmptyToolsPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/emptytools")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["tools"],
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
}

// TestSubagentSyncToolResult: parent turn uses run_subagent tool; child Session is
// independent; parent tool_result contains the child final assistant text.
func TestSubagentSyncToolResult(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildEmptyToolsPluginDir(t, root, pluginsDir, "emptytools")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","emptytools"]}`)

	// Fake LLM: first hop with tools → tool_call to first tool (run_subagent injected).
	// Input "SUBAGENT_TASK" becomes the subagent prompt.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "SUBAGENT_TASK",
		"-session-derive",
		"-session-query",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subagent turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	// Parent must observe a run_subagent tool call.
	if !strings.Contains(s, "run_subagent") {
		t.Fatalf("want run_subagent tool call: %s", s)
	}
	// Child turn final assistant (fake-llm, no child tools) is returned as tool_result.
	if !strings.Contains(s, "You said: SUBAGENT_TASK") {
		t.Fatalf("want child assistant as parent tool_result content: %s", s)
	}
	// Parent must observe a tool role fact.
	if !strings.Contains(s, `"role":"tool"`) && !strings.Contains(s, `"role": "tool"`) {
		t.Fatalf("want tool role in parent derive: %s", s)
	}
}

// TestSubagentAsyncRejected: mode=async is not supported in v1 (error lands as tool_result).
func TestSubagentAsyncRejected(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildEmptyToolsPluginDir(t, root, pluginsDir, "emptytools")
	bin := buildPkg(t, root, "./plugins/asyncsubllm")
	dst := filepath.Join(pluginsDir, "asyncsubllm", "asyncsubllm.exe")
	writeFile(t, filepath.Join(pluginsDir, "asyncsubllm", "plugin.json"), `{
		"name": "asyncsubllm",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["llm"],
		"consumes": [],
		"entry": "asyncsubllm.exe"
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","asyncsubllm","emptytools"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "try async",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	// Tool errors are logged as tool_result and the turn still completes.
	if err != nil {
		t.Fatalf("async subagent turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "not supported") && !strings.Contains(s, "not_supported") {
		t.Fatalf("want async not_supported in tool result: %s", s)
	}
}
