package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionAppendDerive(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

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
	hostBin := buildPkg(t, root, "./cmd/host")

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
	hostBin := buildPkg(t, root, "./cmd/host")

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
	hostBin := buildPkg(t, root, "./cmd/host")

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
	hostBin := buildPkg(t, root, "./cmd/host")

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
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("empty claim should rebuild from log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "agent/request ok") {
		t.Fatalf("want agent/request ok for empty rebuild: %s", out)
	}
}
