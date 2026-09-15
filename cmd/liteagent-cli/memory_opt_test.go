package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionDeriveToolResultStub: log keeps full tool_result facts; derive stubs
// all but the newest fullToolResults (ADR-0015).
func TestSessionDeriveToolResultStub(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	writeFile(t, filepath.Join(pluginsDir, "session", "config.json"),
		`{"fullToolResults":2}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	facts := []map[string]any{
		{"type": "message", "role": "user", "content": "u0"},
		{"type": "tool_call", "role": "assistant", "meta": map[string]any{
			"tool_call_id": "c1", "name": "read_file",
		}},
		{"type": "tool_result", "role": "tool", "content": "OLDEST_TOOL_BODY", "meta": map[string]any{
			"tool_call_id": "c1",
		}},
		{"type": "tool_call", "role": "assistant", "meta": map[string]any{
			"tool_call_id": "c2", "name": "read_file",
		}},
		{"type": "tool_result", "role": "tool", "content": "MID_TOOL_BODY", "meta": map[string]any{
			"tool_call_id": "c2",
		}},
		{"type": "tool_call", "role": "assistant", "meta": map[string]any{
			"tool_call_id": "c3", "name": "read_file",
		}},
		{"type": "tool_result", "role": "tool", "content": "NEWEST_TOOL_BODY", "meta": map[string]any{
			"tool_call_id": "c3",
		}},
	}
	raw, _ := json.Marshal(facts)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", string(raw),
		"-session-derive",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("append+derive: %v\n%s", err, out)
	}
	s := string(out)
	msgs := parseDeriveMessages(t, s)
	stubbed, full := 0, 0
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, "truncated tool result") {
			stubbed++
			continue
		}
		full++
		if m.Content != "MID_TOOL_BODY" && m.Content != "NEWEST_TOOL_BODY" {
			t.Fatalf("unexpected full tool body in derive: %q", m.Content)
		}
	}
	if stubbed != 1 || full != 2 {
		t.Fatalf("want 1 stub + 2 full tool results, got stub=%d full=%d\n%s", stubbed, full, s)
	}
	// Session Log still has the oldest full body.
	if !strings.Contains(s, "OLDEST_TOOL_BODY") {
		t.Fatalf("Session Log must retain full tool result: %s", s)
	}
}

// TestLLMInfoContextWindow: llm.openai exposes contextWindow via llm.info (user-overridable).
func TestLLMInfoContextWindow(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildLLMOpenAIPluginDir(t, root, pluginsDir, "llm-openai", "http://127.0.0.1:9", "deepseek-chat")
	cfgPath := filepath.Join(pluginsDir, "llm-openai", "config.json")
	raw, _ := os.ReadFile(cfgPath)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["contextWindow"] = 12345
	raw, _ = json.Marshal(m)
	writeFile(t, cfgPath, string(raw))

	assembly := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, assembly, `{"plugins":["llm-openai"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", assembly,
		"-invoke", "llm-openai",
		"-frame-cap", "llm",
		"-frame-method", "info",
		"-invoke-payload", `{}`,
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm.info: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "12345") {
		t.Fatalf("want contextWindow 12345: %s", s)
	}
}

// TestPrepareCompactHint: prepare returns compactHint when estimated tokens exceed 0.80 * window.
func TestPrepareCompactHint(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{}`)
	assembly := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, assembly, `{"plugins":["session","context-manager"]}`)

	// ~4000 chars → ~1000 est tokens; window 1000 → threshold 800 → suggest.
	long := strings.Repeat("x", 4000)
	payload := fmt.Sprintf(`{"sessionId":"","messages":[{"role":"user","content":%q}],"contextWindow":1000}`, long)
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", assembly,
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
	if !strings.Contains(s, "compactHint") && !strings.Contains(s, "suggestCompact") {
		t.Fatalf("want compactHint in prepare result: %s", s)
	}
	if !strings.Contains(s, "true") {
		t.Fatalf("want suggestCompact true: %s", s)
	}
}

// TestRefreshBroadcastsConfigReload: /refresh help and success mention config.reload.
func TestRefreshBroadcastsConfigReload(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	assembly := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, assembly, `{"plugins":["session"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, assembly, []string{"/refresh", "/help", "/exit"})
	if !strings.Contains(out, "config.reload") {
		t.Fatalf("want refresh/help to mention config.reload:\n%s", out)
	}
}

// TestLLMOpenAIConfigSetPersistsModel: config set writes disk; config get reflects it.
func TestLLMOpenAIConfigSetPersistsModel(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildLLMOpenAIPluginDir(t, root, pluginsDir, "llm-openai", "http://127.0.0.1:9", "old-model")
	assembly := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, assembly, `{"plugins":["llm-openai"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, assembly, []string{
		"/llm-openai config set model=new-model contextWindow=2048",
		"/llm-openai config get",
		"/exit",
	})
	if !strings.Contains(out, "model=new-model") {
		t.Fatalf("want model=new-model after set:\n%s", out)
	}
	if !strings.Contains(out, "contextWindow=2048") {
		t.Fatalf("want contextWindow=2048 after set:\n%s", out)
	}
}
