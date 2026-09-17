package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestContextCompactProducesSummary: context.compact returns summary text and coversThroughSeq.
func TestContextCompactProducesSummary(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","context-manager","agent"]}`)

	payload := `{"messages":[
		{"role":"user","content":"question one"},
		{"role":"assistant","content":"answer one"},
		{"role":"user","content":"question two"}
	],"coversThroughSeq":3}`
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "context-manager",
		"-frame-cap", "context",
		"-frame-method", "compact",
		"-invoke-payload", payload,
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("invoke context.compact: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, "summary") || !strings.Contains(s, "coversThroughSeq") {
		t.Fatalf("want summary and coversThroughSeq: %s", s)
	}
	if !strings.Contains(s, "question one") {
		t.Fatalf("summary should mention early messages: %s", s)
	}
}

// TestContextRegisterSkillAppearsInPrepare: registerSkill lands in assembled System Prompt.
func TestContextRegisterSkillAppearsInPrepare(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","context-manager","agent"]}`)

	// Same Host process: register skill, then turn so assemble includes the catalog.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "context-manager",
		"-frame-cap", "context",
		"-frame-method", "registerSkill",
		"-invoke-payload", `{"name":"demo-skill","text":"SKILL_DEMO_CATALOG"}`,
		"-turn", "hi",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("registerSkill+turn: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SKILL_DEMO_CATALOG") {
		t.Fatalf("want skill catalog in System Prompt/derive: %s", out)
	}
}

// TestContextUsageCharsAreModelVisibleOnly: usage.chars must equal sum of
// message content (not systemText again, not Host/JSON framing).
func TestContextUsageCharsAreModelVisibleOnly(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [{"name":"identity","order":-1000,"text":"SYS_ONLY_XXXX"}]
	}`)
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","context-manager","agent"]}`)

	// 5-char user content only → chars must be 5 (not 5+len(SYS_ONLY_XXXX)).
	payload := `{"sessionId":"","messages":[{"role":"user","content":"hello"}]}`
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "context-manager",
		"-frame-cap", "context",
		"-frame-method", "prepare",
		"-invoke-payload", payload,
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, `"chars":5`) && !strings.Contains(s, `"chars": 5`) {
		t.Fatalf("want usage.chars==5 (model-visible content only): %s", s)
	}
}
