package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLoopOneTurn(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello loop",
		"-session-derive",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("one turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if !strings.Contains(s, "hello loop") {
		t.Fatalf("want user input in turn output: %s", s)
	}
	// fake-llm must stream chunks then produce a full assistant reply.
	if !strings.Contains(s, "chunk") {
		t.Fatalf("want streamed llm chunks observed: %s", s)
	}
	// Assistant reply derived from Session Log after the turn.
	if !strings.Contains(s, "derive ok") {
		t.Fatalf("want derive after turn: %s", s)
	}
	if !strings.Contains(s, "assistant") {
		t.Fatalf("want assistant message in derived Model Context: %s", s)
	}
	if !strings.Contains(s, "You said:") {
		t.Fatalf("want fake-llm assistant content in derive: %s", s)
	}
}

func TestDefaultLoopRequiresLLM(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want failure when no llm provider: %s", out)
	}
	if !strings.Contains(string(out), "llm") {
		t.Fatalf("want llm missing diagnosis: %s", out)
	}
}
