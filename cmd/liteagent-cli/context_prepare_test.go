package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestContextPrepareReturnsSystemAndTools: context.prepare returns assembled systemText
// and tools schemas collected from the mounted tools plugin.
func TestContextPrepareReturnsSystemAndTools(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [
			{"name": "identity", "order": -1000, "text": "You are CM_PREPARE_IDENTITY."}
		]
	}`)
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","context-manager","echotool"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "context-manager",
		"-frame-cap", "context",
		"-frame-method", "prepare",
		"-invoke-payload", `{"sessionId":"","messages":[{"role":"user","content":"hello"}]}`,
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("invoke context.prepare: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, "CM_PREPARE_IDENTITY") {
		t.Fatalf("want systemText from segments: %s", s)
	}
	if !strings.Contains(s, "echo_text") {
		t.Fatalf("want tools schemas from tools.list: %s", s)
	}
}

// TestContextCompactProducesSummary: context.compact returns summary text and coversThroughSeq.
func TestContextCompactProducesSummary(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","context-manager"]}`)

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
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","context-manager"]}`)

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
