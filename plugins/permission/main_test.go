package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecideDefaultAllow(t *testing.T) {
	action, _ := decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), t.TempDir())
	if action != "allow" {
		t.Fatalf("want allow, got %s", action)
	}
}

func TestDecideProjectDenyOverrides(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".liteagent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rules := `{"defaultAction":"allow","rules":[{"tool":"shell","action":"deny"},{"tool":"write_file","path":"**","action":"ask"}]}`
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	action, reason := decide("shell", json.RawMessage(`{"command":"rm -rf /"}`), ws)
	if action != "deny" {
		t.Fatalf("want deny shell, got %s (%s)", action, reason)
	}
	action, _ = decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), ws)
	if action != "ask" {
		t.Fatalf("want ask write_file, got %s", action)
	}
	action, _ = decide("read_file", json.RawMessage(`{"path":"src/a.go"}`), ws)
	if action != "allow" {
		t.Fatalf("want allow read_file, got %s", action)
	}
}
