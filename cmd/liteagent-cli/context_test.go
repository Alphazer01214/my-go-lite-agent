package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdditionalContextsAfterToolResult(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","echotool","agent"]}`)

	// "ctx" makes echotool return content + additionalContexts.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "ctx",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn with additionalContexts: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if !strings.Contains(s, "ADDITIONAL_CTX_MARKER") {
		t.Fatalf("want additionalContexts in derived Model Context: %s", s)
	}
	// Order: tool result before additional context.
	deriveIdx := strings.Index(s, "derive ok messages=")
	if deriveIdx < 0 {
		t.Fatalf("want derive ok: %s", s)
	}
	derived := s[deriveIdx:]
	toolIdx := strings.Index(derived, `"role":"tool"`)
	ctxIdx := strings.Index(derived, "ADDITIONAL_CTX_MARKER")
	if toolIdx < 0 || ctxIdx < 0 || !(toolIdx < ctxIdx) {
		t.Fatalf("want tool_result before additionalContexts: %s", derived)
	}
}

func TestAgentInjectAppendsWithoutTurn(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-agent-inject", `[{"role":"system","content":"INJECTED_NOTE"}]`,
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent inject: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "inject ok") {
		t.Fatalf("want inject ok: %s", s)
	}
	if !strings.Contains(s, "INJECTED_NOTE") {
		t.Fatalf("want injected content in Model Context: %s", s)
	}
	// Must not start a turn (no llm mounted; turn would fail or print turn ok).
	if strings.Contains(s, "turn ok") {
		t.Fatalf("inject must not wake a turn: %s", s)
	}
}

func TestPluginAgentInjectViaStar(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildAgentProbePluginDir(t, root, pluginsDir, "agentprobe")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","agentprobe","agent"]}`)

	// agentprobe demo.inject →agent.inject through Host star.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "agentprobe",
		"-call-cap", "inject",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin agent.inject: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, "PLUGIN_INJECTED") {
		t.Fatalf("want plugin-injected content in Model Context: %s", s)
	}
	if strings.Contains(s, "turn ok") {
		t.Fatalf("plugin inject must not start a turn: %s", s)
	}
}
