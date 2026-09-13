package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresentationCardEmittedFromTool(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","echotool"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "card-me",
		"-cards",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn with cards: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if !strings.Contains(s, "card[0]") {
		t.Fatalf("want at least one presentation card: %s", s)
	}
	if !strings.Contains(s, "type=echo_result") {
		t.Fatalf("want identifiable cardType: %s", s)
	}
	if !strings.Contains(s, "tool=echo_text") {
		t.Fatalf("want tool name on card: %s", s)
	}
	if !strings.Contains(s, "card-me") {
		t.Fatalf("want card data projected from args/result: %s", s)
	}
}

func TestPresentationCardReplayDeterministic(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	run := func() string {
		t.Helper()
		pluginsDir := t.TempDir()
		buildSessionPluginDir(t, root, pluginsDir, "session")
		buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
		buildEchoToolPluginDir(t, root, pluginsDir, "echotool")
		cfg := filepath.Join(t.TempDir(), "assembly.json")
		writeFile(t, cfg, `{"plugins":["session","fakellm","echotool"]}`)
		out, err := exec.Command(hostBin,
			"-plugins", pluginsDir,
			"-assembly", cfg,
			"-turn", "same-input",
			"-cards",
		).CombinedOutput()
		if err != nil {
			t.Fatalf("replay run: %v\n%s", err, out)
		}
		s := string(out)
		idx := strings.Index(s, "card[0]")
		if idx < 0 {
			t.Fatalf("want card[0] in output: %s", s)
		}
		return s[idx:]
	}

	a, b := run(), run()
	if a != b {
		t.Fatalf("same tool result must yield same card on replay:\nA: %s\nB: %s", a, b)
	}
}

func TestFunctionWithoutPresentationStillWorks(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	// No tools plugin: Function/LLM path without Presentation face.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "no card needed",
		"-cards",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn without presentation: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if strings.Contains(s, "card[0]") {
		t.Fatalf("must not invent cards without presentation: %s", s)
	}
}
