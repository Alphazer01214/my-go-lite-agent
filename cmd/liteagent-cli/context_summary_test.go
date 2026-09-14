package main_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionDeriveSingleSystemPrompt: multiple role=system message facts remain in the log,
// but Model Context projects only the last one (in place; earlier systems dropped).
func TestSessionDeriveSingleSystemPrompt(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	appendJSON := `[
		{"type":"message","role":"system","content":"SYS_OLD"},
		{"type":"message","role":"user","content":"u1"},
		{"type":"message","role":"system","content":"SYS_NEW"},
		{"type":"message","role":"user","content":"u2"}
	]`

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", appendJSON,
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("append+derive: %v\n%s", err, out)
	}
	s := string(out)
	msgs := parseDeriveMessages(t, s)
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages (drop earlier system, keep last in place), got %d: %+v", len(msgs), msgs)
	}
	foundNew := false
	for _, m := range msgs {
		if m.Content == "SYS_OLD" {
			t.Fatalf("derive must not project SYS_OLD: %+v", msgs)
		}
		if m.Content == "SYS_NEW" && m.Role == "system" {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatalf("want last system SYS_NEW in derive: %+v", msgs)
	}
	if !strings.Contains(s, "u1") || !strings.Contains(s, "u2") {
		t.Fatalf("want both users retained: %s", s)
	}
}

// TestSessionDeriveActiveContextSummary: active context_summary replaces covered history.
func TestSessionDeriveActiveContextSummary(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	// seq: 1 sys, 2 user, 3 assistant, 4 user, 5 summary covers 1-4, 6 user after
	appendJSON := `[
		{"type":"message","role":"system","content":"SYS1"},
		{"type":"message","role":"user","content":"old-user"},
		{"type":"message","role":"assistant","content":"old-assistant"},
		{"type":"message","role":"user","content":"old-user-2"},
		{"type":"context_summary","role":"system","content":"SUMMARY_MARK","meta":{"active":true,"coversThroughSeq":4}},
		{"type":"message","role":"user","content":"fresh-user"}
	]`

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", appendJSON,
		"-session-derive",
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("append+derive: %v\n%s", err, out)
	}
	s := string(out)
	deriveLine := ""
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "derive ok messages=") {
			deriveLine = strings.TrimSpace(line)
			break
		}
	}
	if deriveLine == "" {
		t.Fatalf("want derive ok: %s", s)
	}
	if !strings.Contains(deriveLine, "SUMMARY_MARK") {
		t.Fatalf("want active summary in derive: %s", deriveLine)
	}
	if !strings.Contains(deriveLine, "fresh-user") {
		t.Fatalf("want post-cover user in derive: %s", deriveLine)
	}
	for _, banned := range []string{"old-user-2", "old-assistant", `"old-user"`} {
		if strings.Contains(deriveLine, banned) {
			t.Fatalf("covered fact %s must not project: %s", banned, deriveLine)
		}
	}
	// Full log still has originals (append-only).
	if !strings.Contains(s, "query ok") || !strings.Contains(s, "old-assistant") {
		t.Fatalf("Session Log must retain covered facts: %s", s)
	}

	msgs := parseDeriveMessages(t, s)
	foundSummary := false
	foundFresh := false
	for _, m := range msgs {
		if m.Content == "SUMMARY_MARK" {
			foundSummary = true
		}
		if m.Content == "fresh-user" {
			foundFresh = true
		}
		if m.Content == "SYS1" || m.Content == "old-user" || m.Content == "old-assistant" {
			t.Fatalf("derive leaked covered content %q: %+v", m.Content, msgs)
		}
	}
	if !foundSummary || !foundFresh {
		t.Fatalf("want summary+fresh in msgs: %+v", msgs)
	}
}

type deriveMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func parseDeriveMessages(t *testing.T, s string) []deriveMsg {
	t.Helper()
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "derive ok messages=") {
			continue
		}
		body := strings.TrimPrefix(line, "derive ok messages=")
		var msgs []deriveMsg
		if err := json.Unmarshal([]byte(body), &msgs); err != nil {
			t.Fatalf("parse derive: %v\n%s", err, body)
		}
		return msgs
	}
	t.Fatalf("no derive ok line in output:\n%s", s)
	return nil
}
