package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func validInfo() json.RawMessage {
	return json.RawMessage(`{
		"capability": "file",
		"tools": [
			{
				"name": "file_read",
				"description": "Read a file.",
				"parameters": {"type": "object", "properties": {"path": {"type": "string"}}},
				"method": "read"
			}
		]
	}`)
}

func TestValidateProviderInfo(t *testing.T) {
	tools, diagnostic := validateProviderInfo("file", validInfo())
	if diagnostic != nil {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	if tools[0].Capability != "file" || tools[0].Method != "read" {
		t.Fatalf("tool = %+v", tools[0])
	}
}

func TestValidateProviderInfoCapabilityMismatch(t *testing.T) {
	_, diagnostic := validateProviderInfo("shell", validInfo())
	if diagnostic == nil || diagnostic.Code != "capability_mismatch" {
		t.Fatalf("diagnostic = %+v, want capability_mismatch", diagnostic)
	}
}

func TestValidateProviderInfoRejectsInvalidDefinition(t *testing.T) {
	cases := []string{
		`{"capability":"file","tools":[{"name":"Bad-Name","description":"x","parameters":{"type":"object"},"method":"read"}]}`,
		`{"capability":"file","tools":[{"name":"file_read","description":"","parameters":{"type":"object"},"method":"read"}]}`,
		`{"capability":"file","tools":[{"name":"file_read","description":"x","parameters":{"type":"string"},"method":"read"}]}`,
		`{"capability":"file","tools":[{"name":"file_read","description":"x","parameters":{"type":"object"},"method":"info"}]}`,
		`{"capability":"file","tools":[{"name":"file_read","description":"x","parameters":{"type":"object"},"method":"read"},{"name":"file_read","description":"y","parameters":{"type":"object"},"method":"read"}]}`,
	}
	for _, raw := range cases {
		if _, diagnostic := validateProviderInfo("file", json.RawMessage(raw)); diagnostic == nil || diagnostic.Code != "definition_invalid" {
			t.Fatalf("raw=%s diagnostic=%+v, want definition_invalid", raw, diagnostic)
		}
	}
}

func TestNormalizeObject(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		value, err := normalizeObject(json.RawMessage(raw), "arguments")
		if err != nil || string(value) != "{}" {
			t.Fatalf("raw=%q value=%s err=%v", raw, value, err)
		}
	}
	if _, err := normalizeObject(json.RawMessage(`[]`), "arguments"); err == nil {
		t.Fatal("array should be rejected")
	}
}

func TestValidateToolResult(t *testing.T) {
	if err := validateToolResult(json.RawMessage(`{"content":"ok","is_error":false,"truncated":false}`)); err != nil {
		t.Fatalf("valid result: %v", err)
	}
	if err := validateToolResult(json.RawMessage(`{"content":"ok","meta":{"code":"x"}}`)); err != nil {
		t.Fatalf("valid meta: %v", err)
	}
	for _, raw := range []string{
		`{"is_error":false}`,
		`{"content":null}`,
		`{"content":"ok","is_error":"no"}`,
		`{"content":"ok","meta":[]}`,
		`[]`,
	} {
		if err := validateToolResult(json.RawMessage(raw)); err == nil {
			t.Fatalf("raw=%s should be rejected", raw)
		}
	}
	if err := validateToolResult(json.RawMessage(`{"content":"` + strings.Repeat("x", maxContentBytes+1) + `"}`)); err == nil {
		t.Fatal("oversized content should be rejected")
	}
}
