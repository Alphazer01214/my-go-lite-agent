package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
