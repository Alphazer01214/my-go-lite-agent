package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandSkillsInjectsKnownAndKeepsMissing(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".liteagent", "skills", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# review\n\nReview checklist: tests, docs\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, injected, missing := expandSkills(ws, "please $review this and also $nope")
	if len(injected) != 1 || injected[0] != "review" {
		t.Fatalf("injected=%v", injected)
	}
	if len(missing) != 1 || missing[0] != "nope" {
		t.Fatalf("missing=%v", missing)
	}
	if !strings.Contains(out, "Review checklist") {
		t.Fatalf("out=%q", out)
	}
	if !strings.Contains(out, "$nope") {
		t.Fatalf("missing token should remain: %q", out)
	}
}
