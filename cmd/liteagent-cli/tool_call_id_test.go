package main_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionDeriveKeepsToolCallID: Host writes meta.tool_calls[].tool_call_id;
// derive must project it as ToolCall.id (OpenAI requires non-empty unique ids).
func TestSessionDeriveKeepsToolCallID(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	appendJSON := `[{"type":"tool_call","role":"assistant","content":"","meta":{"tool_calls":[{"tool_call_id":"call_abc","name":"read_file","arguments":{"path":"a.txt"}}]}},{"type":"tool_result","role":"tool","content":"ok","meta":{"tool_call_id":"call_abc"}}]`

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", appendJSON,
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("append+derive: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "call_abc") {
		t.Fatalf("want tool_call_id projected as id: %s", s)
	}
	// Parse derive line to ensure id field is populated.
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "derive ok messages=") {
			continue
		}
		body := strings.TrimPrefix(line, "derive ok messages=")
		var msgs []struct {
			ToolCalls []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
			ToolCallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal([]byte(body), &msgs); err != nil {
			t.Fatalf("parse derive: %v\n%s", err, body)
		}
		foundCall := false
		foundResult := false
		for _, m := range msgs {
			for _, tc := range m.ToolCalls {
				if tc.ID == "call_abc" {
					foundCall = true
				}
			}
			if m.ToolCallID == "call_abc" {
				foundResult = true
			}
		}
		if !foundCall {
			t.Fatalf("want assistant tool_call id=call_abc: %s", body)
		}
		if !foundResult {
			t.Fatalf("want tool result tool_call_id=call_abc: %s", body)
		}
	}
}

// TestChatAssemblyInjectsNoToolsNote: without a tools plugin, Loop must tell the model tools are unavailable.
func TestChatAssemblyInjectsNoToolsNote(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildContextManagerPluginDir(t, root, pluginsDir, "contextmanager", `{
		"segments":[{"name":"identity","order":-1000,"text":"You are a lite agent."}]
	}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","contextmanager"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "What's the current workspace",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("chat turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "No tools are mounted") {
		t.Fatalf("want no-tools system note in Model Context: %s", s)
	}
}
