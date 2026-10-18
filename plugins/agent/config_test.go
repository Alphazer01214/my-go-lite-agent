package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilterTools(t *testing.T) {
	tools := []toolSpec{{Name: "read_file"}, {Name: "write_file"}, {Name: "grep"}}
	got := filterTools(tools, schemeChat, nil)
	if len(got) != 0 {
		t.Fatalf("chat: %+v", got)
	}
	got = filterTools(tools, schemeToolCalling, nil)
	if len(got) != 3 {
		t.Fatalf("tool_calling: %+v", got)
	}
	got = filterTools(tools, schemeChat, []string{"read_file", "grep"})
	if len(got) != 2 {
		t.Fatalf("allowed overrides scheme: %+v", got)
	}
}

func TestTurnStateCancel(t *testing.T) {
	st := newTurnState(context.Background(), "s1")
	if st.isCancelled() {
		t.Fatal("fresh")
	}
	if !st.markCancelled() {
		t.Fatal("first cancel")
	}
	if st.markCancelled() {
		t.Fatal("idempotent")
	}
	if !st.isCancelled() {
		t.Fatal("flag")
	}
}

func TestConfigSetAppliesToSubsequentTurns(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	before := p.snapshot()
	out, err := p.configSet(configSetIn{DefaultScheme: "coding", MaxSteps: 16})
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScheme != "coding" || out.MaxSteps != 16 {
		t.Fatalf("out=%+v", out)
	}
	if before.DefaultScheme == "coding" {
		t.Fatal("captured snapshot must not change")
	}
	if p.snapshot().DefaultScheme != "coding" || p.snapshot().MaxSteps != 16 {
		t.Fatal("new snapshot must use new config")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal("config not written")
	}
}

func TestMaxStepsDefault(t *testing.T) {
	cfg := defaultConfig()
	if cfg.MaxSteps != 128 {
		t.Fatalf("default max_steps=%d", cfg.MaxSteps)
	}
}
