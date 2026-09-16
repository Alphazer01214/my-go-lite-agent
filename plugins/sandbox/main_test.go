package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecideDefaultAllow(t *testing.T) {
	action, _ := decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), t.TempDir(), "")
	if action != "allow" {
		t.Fatalf("want allow, got %s", action)
	}
}

func TestDecideSeverityAskWhenNoRule(t *testing.T) {
	// No permissions.json: severity medium/high should ask by default policy.
	action, reason := decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), t.TempDir(), "medium")
	if action != "ask" {
		t.Fatalf("want ask for medium severity, got %s (%s)", action, reason)
	}
	action, _ = decide("shell", json.RawMessage(`{"command":"ls"}`), t.TempDir(), "high")
	if action != "ask" {
		t.Fatalf("want ask for high severity, got %s", action)
	}
	action, _ = decide("read_file", json.RawMessage(`{"path":"a.go"}`), t.TempDir(), "low")
	if action != "allow" {
		t.Fatalf("want allow for low severity, got %s", action)
	}
}

func TestDecideRuleBeatsSeverity(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".liteagent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rules := `{"defaultAction":"allow","rules":[{"tool":"shell","action":"allow"}]}`
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	action, reason := decide("shell", json.RawMessage(`{"command":"ls"}`), ws, "high")
	if action != "allow" {
		t.Fatalf("explicit rule must beat severity, got %s (%s)", action, reason)
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
	action, reason := decide("shell", json.RawMessage(`{"command":"rm -rf /"}`), ws, "high")
	if action != "deny" {
		t.Fatalf("want deny shell, got %s (%s)", action, reason)
	}
	action, _ = decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), ws, "medium")
	if action != "ask" {
		t.Fatalf("want ask write_file, got %s", action)
	}
	action, _ = decide("read_file", json.RawMessage(`{"path":"src/a.go"}`), ws, "low")
	if action != "allow" {
		t.Fatalf("want allow read_file, got %s", action)
	}
}

func TestConfigSetSeverityPolicy(t *testing.T) {
	raw, err := handleConfigCap("set", json.RawMessage(`{"severityPolicy":{"high":"deny"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	pol, _ := out["severityPolicy"].(map[string]any)
	if pol == nil || pol["high"] != "deny" {
		t.Fatalf("want high=deny, got %v", out)
	}
	// restore
	if _, err := handleConfigCap("set", json.RawMessage(`{"severityPolicy":{"high":"ask"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSetRejectsInvalidDefaultAction(t *testing.T) {
	if _, err := handleConfigCap("set", json.RawMessage(`{"defaultAction":"nope"}`)); err == nil {
		t.Fatal("want bad_assignment for invalid defaultAction")
	}
}

func TestConfigSchemaExposesDefaultAction(t *testing.T) {
	raw, err := handleConfigCap("schema", nil)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Fields []map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil || len(schema.Fields) == 0 {
		t.Fatalf("schema fields missing: %v", err)
	}
	if schema.Fields[0]["name"] != "defaultAction" {
		t.Fatalf("want defaultAction field, got %v", schema.Fields[0])
	}
}

func TestConfigSetValidDefaultAction(t *testing.T) {
	raw, err := handleConfigCap("set", json.RawMessage(`{"defaultAction":"ask"}`))
	if err != nil {
		t.Fatalf("set ask: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("want ok, got %v", out)
	}
	if _, err := handleConfigCap("set", json.RawMessage(`{"defaultAction":"allow"}`)); err != nil {
		t.Fatalf("restore allow: %v", err)
	}
}
