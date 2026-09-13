package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolCallPathOneTurn(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","echotool"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "please echo me",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tool turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if !strings.Contains(s, "tool_call") {
		t.Fatalf("want tool_call observed in turn: %s", s)
	}
	if !strings.Contains(s, "please echo me") {
		t.Fatalf("want user input in output: %s", s)
	}
	// Final assistant reply after tool result.
	if !strings.Contains(s, "Tool said:") {
		t.Fatalf("want assistant reply reflecting tool result: %s", s)
	}
	if !strings.Contains(s, "derive ok") {
		t.Fatalf("want derive after turn: %s", s)
	}
	// Model Context: user → assistant tool_call → tool result → final assistant.
	if !strings.Contains(s, `"role":"tool"`) {
		t.Fatalf("want tool role message in derived Model Context: %s", s)
	}
	if !strings.Contains(s, "echo_text") {
		t.Fatalf("want tool name in derived Model Context: %s", s)
	}
	if !strings.Contains(s, "tool_calls") {
		t.Fatalf("want tool_calls projected from session log: %s", s)
	}
	// Extract derive payload and assert fact order inside it.
	deriveIdx := strings.Index(s, "derive ok messages=")
	if deriveIdx < 0 {
		t.Fatalf("want derive ok messages= in output: %s", s)
	}
	derived := s[deriveIdx:]
	userIdx := strings.Index(derived, `"role":"user"`)
	callIdx := strings.Index(derived, `"tool_calls"`)
	toolRoleIdx := strings.Index(derived, `"role":"tool"`)
	finalIdx := strings.Index(derived, `"Tool said:`)
	if userIdx < 0 || callIdx < 0 || toolRoleIdx < 0 || finalIdx < 0 ||
		!(userIdx < callIdx && callIdx < toolRoleIdx && toolRoleIdx < finalIdx) {
		t.Fatalf("want user → tool_call → tool result → final assistant order in derive: %s", derived)
	}
}

func TestToolCallWithoutToolPluginStillAnswers(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	// No tools plugin: fake-llm must still answer without tool_calls.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello tools",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn without tools: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if strings.Contains(s, "tool_call") {
		t.Fatalf("must not invent tool calls without tools plugin: %s", s)
	}
	if !strings.Contains(s, "You said:") {
		t.Fatalf("want normal assistant reply: %s", s)
	}
}

func TestToolCallErrorStillCompletesTurn(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","echotool"]}`)

	// "boom" makes echotool fail; Loop must log error tool_result and continue.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "boom",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tool error must not abort turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok after tool error: %s", s)
	}
	if !strings.Contains(s, "error:") && !strings.Contains(s, "tool_failed") && !strings.Contains(s, "refused") {
		t.Fatalf("want tool error visible in log/output: %s", s)
	}
	if !strings.Contains(s, "Tool said:") {
		t.Fatalf("want final assistant after tool error: %s", s)
	}
	if !strings.Contains(s, `"role":"tool"`) {
		t.Fatalf("want tool_result fact in Model Context: %s", s)
	}
}
