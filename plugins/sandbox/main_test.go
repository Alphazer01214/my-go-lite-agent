package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecideDefaultAllow(t *testing.T) {
	action, _ := decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), t.TempDir(), "", "workspace_write")
	if action != "allow" {
		t.Fatalf("want allow, got %s", action)
	}
}

func TestModeProfileReadOnly(t *testing.T) {
	ws := t.TempDir()
	action, reason := decide("read_file", json.RawMessage(`{"path":"a.go"}`), ws, "low", "read_only")
	if action != "allow" {
		t.Fatalf("read_only low: %s (%s)", action, reason)
	}
	action, reason = decide("write_file", json.RawMessage(`{"path":"a.go"}`), ws, "medium", "read_only")
	if action != "deny" {
		t.Fatalf("read_only medium: %s (%s)", action, reason)
	}
	action, _ = decide("shell", json.RawMessage(`{"command":"ls"}`), ws, "high", "read_only")
	if action != "deny" {
		t.Fatalf("read_only high: %s", action)
	}
}

func TestModeProfileWorkspaceWrite(t *testing.T) {
	ws := t.TempDir()
	action, reason := decide("write_file", json.RawMessage(`{"path":"`+filepath.Join(ws, "a.go")+`"}`), ws, "medium", "workspace_write")
	if action != "allow" {
		t.Fatalf("ws write inside: %s (%s)", action, reason)
	}
	action, reason = decide("write_file", json.RawMessage(`{"path":"/etc/hosts"}`), ws, "medium", "workspace_write")
	if action != "deny" {
		t.Fatalf("ws write outside: %s (%s)", action, reason)
	}
	action, _ = decide("shell", json.RawMessage(`{"command":"ls"}`), ws, "high", "workspace_write")
	if action != "ask" {
		t.Fatalf("ws high shell: %s", action)
	}
}

func TestModeProfileFullAccessUsesSeverityPolicy(t *testing.T) {
	ws := t.TempDir()
	// full_access: modeProfile declines; severityPolicy medium→ask by default.
	action, reason := decide("write_file", json.RawMessage(`{"path":"x"}`), ws, "medium", "full_access")
	if action != "ask" {
		t.Fatalf("full_access medium: %s (%s)", action, reason)
	}
}

func TestModeExplicitAllowOverridesProfile(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".liteagent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rules := `{"rules":[{"tool":"write_file","action":"allow"}]}`
	if err := os.WriteFile(filepath.Join(dir, "permissions.json"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	// read_only would deny write, but explicit allow wins (ADR-0033 default profile).
	action, reason := decide("write_file", json.RawMessage(`{"path":"/tmp/x"}`), ws, "medium", "read_only")
	if action != "allow" {
		t.Fatalf("explicit allow must widen mode: %s (%s)", action, reason)
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
	action, reason := decide("shell", json.RawMessage(`{"command":"ls"}`), ws, "high", "workspace_write")
	if action != "allow" {
		t.Fatalf("explicit rule must beat severity/mode, got %s (%s)", action, reason)
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
	action, reason := decide("shell", json.RawMessage(`{"command":"rm -rf /"}`), ws, "high", "full_access")
	if action != "deny" {
		t.Fatalf("want deny shell, got %s (%s)", action, reason)
	}
	action, _ = decide("write_file", json.RawMessage(`{"path":"src/a.go"}`), ws, "medium", "full_access")
	if action != "ask" {
		t.Fatalf("want ask write_file, got %s", action)
	}
	action, _ = decide("read_file", json.RawMessage(`{"path":"src/a.go"}`), ws, "low", "full_access")
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
