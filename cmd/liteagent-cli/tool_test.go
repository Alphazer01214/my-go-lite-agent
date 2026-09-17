package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolCallWithoutToolPluginStillAnswers(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","agent"]}`)

	// No tools plugin: fake-llm must still answer without tool_calls.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello tools",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
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
