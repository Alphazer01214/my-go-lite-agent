package main_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSessionFactsCarryTimestamp(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"role":"user","content":"ts-check"}]`,
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session query: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "ts-check") {
		t.Fatalf("want fact content: %s", s)
	}
	// query JSON includes ts (UnixMilli) on each fact.
	if !strings.Contains(s, `"ts"`) {
		t.Fatalf("want ts field on session facts: %s", s)
	}
}

// TestSessionAppendHonorsCallerTs: session.append keeps a positive caller ts
// (e.g. llm-openai reasoning hop-end stamp) instead of overwriting with now.
func TestSessionAppendHonorsCallerTs(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	const wantTs = 1700000000123
	appendJSON := `[{"type":"reasoning","role":"assistant","content":"think-stamp","ts":` +
		strconv.FormatInt(wantTs, 10) + `}]`
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", appendJSON,
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session append/query: %v\n%s", err, out)
	}
	s := string(out)

	var facts []map[string]any
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "query ok facts=") {
			continue
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "query ok facts=")), &facts); err != nil {
			t.Fatalf("parse facts: %v\n%s", err, line)
		}
	}
	var found bool
	for _, f := range facts {
		if f["type"] != "reasoning" || f["content"] != "think-stamp" {
			continue
		}
		found = true
		got, _ := f["ts"].(float64)
		if int64(got) != wantTs {
			t.Fatalf("want caller ts=%d, got %v (%s)", wantTs, f["ts"], s)
		}
	}
	if !found {
		t.Fatalf("want reasoning fact think-stamp: %s", s)
	}
}

func TestSessionAppendDerive(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]`,
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session append/derive: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "append ok") {
		t.Fatalf("want append ok: %s", s)
	}
	if !strings.Contains(s, "derive ok") {
		t.Fatalf("want derive ok: %s", s)
	}
	if !strings.Contains(s, "hello") || !strings.Contains(s, "hi") {
		t.Fatalf("want derived messages to include appended content: %s", s)
	}
}

func TestAgentRequestAcceptsRebuiltContext(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"role":"user","content":"hello"}]`,
		"-agent-request", `[{"role":"user","content":"hello"}]`,
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent request should accept logged context: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "agent/request ok") {
		t.Fatalf("want agent/request ok: %s", s)
	}
	if !strings.Contains(s, "hello") {
		t.Fatalf("want rebuilt messages in result: %s", s)
	}
}

func TestAgentRequestRejectsUnloggedMessages(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	// Log one message, then try to smuggle a different one into the model request.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"role":"user","content":"logged"}]`,
		"-agent-request", `[{"role":"user","content":"not-in-log"}]`,
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want rejection for unlogged model context: %s", out)
	}
	s := string(out)
	if !strings.Contains(s, "session_invariant_violation") && !strings.Contains(s, "reconstructable") {
		t.Fatalf("want session invariant diagnosis: %s", s)
	}
}

func TestAgentRequestRejectsEmptyLogSmuggle(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	// Nothing appended: any claimed messages must be rejected.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-agent-request", `[{"role":"user","content":"ghost"}]`,
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want rejection when session log is empty: %s", out)
	}
	s := string(out)
	if !strings.Contains(s, "session_invariant_violation") && !strings.Contains(s, "reconstructable") {
		t.Fatalf("want session invariant diagnosis: %s", s)
	}
}

func TestAgentRequestEmptyClaimUsesDerived(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	// Empty claimed list means "rebuild from log" — should succeed on empty log.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-agent-request", `[]`,
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("empty claim should rebuild from log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "agent/request ok") {
		t.Fatalf("want agent/request ok for empty rebuild: %s", out)
	}
	if !strings.Contains(string(out), "rebuilt=true") {
		t.Fatalf("want rebuilt=true for empty claim: %s", out)
	}
}

func TestSessionQueryReturnsFacts(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"role":"user","content":"q-me"}]`,
		"-session-query",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session query: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "query ok") {
		t.Fatalf("want query ok: %s", s)
	}
	if !strings.Contains(s, "q-me") {
		t.Fatalf("want queried fact content: %s", s)
	}
	if !strings.Contains(s, `"seq"`) {
		t.Fatalf("want fact seq in query result: %s", s)
	}
}
