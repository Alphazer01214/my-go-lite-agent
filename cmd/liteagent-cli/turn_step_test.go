package main_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTurnStepBoundaryEvents: default Loop logs turn/step boundaries; derive ignores them.
func TestTurnStepBoundaryEvents(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm","echotool"]}`)

	// "hello" with tools → fake-llm issues one tool call → second step final reply.
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hello boundaries",
		"-session-query",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn with boundaries: %v\n%s", err, out)
	}
	s := string(out)

	// Parse query facts from output line query ok facts=...
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
	if len(facts) == 0 {
		t.Fatalf("want session facts: %s", s)
	}

	var turnStart, turnEnd, stepStart, stepEnd int
	for _, f := range facts {
		typ, _ := f["type"].(string)
		switch typ {
		case "turn_start":
			turnStart++
		case "turn_end":
			turnEnd++
		case "step_start":
			stepStart++
		case "step_end":
			stepEnd++
		}
	}
	if turnStart != 1 || turnEnd != 1 {
		t.Fatalf("want 1 turn_start and 1 turn_end, got start=%d end=%d\n%s", turnStart, turnEnd, s)
	}
	// Tool path uses two model hops → two steps.
	if stepStart < 2 || stepEnd < 2 {
		t.Fatalf("want >=2 step_start/step_end, got start=%d end=%d\n%s", stepStart, stepEnd, s)
	}

	// Derive line must not include boundary types.
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "derive ok") {
			continue
		}
		if strings.Contains(line, "turn_start") || strings.Contains(line, "step_start") ||
			strings.Contains(line, "turn_end") || strings.Contains(line, "step_end") {
			t.Fatalf("derive must ignore boundary facts: %s", line)
		}
	}
}
