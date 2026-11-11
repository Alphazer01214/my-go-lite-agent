package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var message struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id,omitempty"`
			Method  string          `json:"method,omitempty"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if err := decoder.Decode(&message); err != nil {
			os.Exit(0)
		}
		if len(message.ID) == 0 || message.Method == "" {
			continue
		}
		var result any
		switch message.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "fake", "version": "1.0.0"},
			}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{{
					"name":        "echo",
					"description": "Echo text",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"text": map[string]any{"type": "string"}},
					},
				}},
			}
		case "tools/call":
			var params struct {
				Name      string `json:"name"`
				Arguments struct {
					Text string `json:"text"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(message.Params, &params)
			if params.Name != "echo" {
				_ = encoder.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      message.ID,
					"error":   map[string]any{"code": -32601, "message": "unknown tool"},
				})
				continue
			}
			result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": "echo: " + params.Arguments.Text}},
			}
		default:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      message.ID,
				"error":   map[string]any{"code": -32601, "message": "unknown method"},
			})
			continue
		}
		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      message.ID,
			"result":  result,
		})
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func mcpTestEntry(t *testing.T) mcpEntry {
	t.Helper()
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatalf("helper path: %v", err)
	}
	return mcpEntry{
		Name:    "fake",
		Enabled: boolPtr(true),
		Type:    mcpTransportStdio,
		Command: command,
		Args:    []string{"-test.run=TestMCPHelperProcess", "--"},
		Env:     map[string]string{"GO_WANT_MCP_HELPER": "1"},
	}
}

func TestMCPInfoAndCall(t *testing.T) {
	cfg := config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		MCPEnabled:     boolPtr(true),
		MCPTimeoutMS:   5000,
		MCPServers:     map[string]mcpEntry{"fake": mcpTestEntry(t)},
	}
	p := &plugin{cfg: cfg}
	p.startMCP()
	defer p.closeMCP()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := p.mcp.snapshot()
		if len(snapshot.Servers) == 1 && snapshot.Servers[0].Status == mcpStatusConnected {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	snapshot := p.mcp.snapshot()
	if len(snapshot.Servers) != 1 || snapshot.Servers[0].Status != mcpStatusConnected {
		t.Fatalf("mcp snapshot = %+v", snapshot)
	}
	if len(snapshot.Servers[0].Tools) != 1 || snapshot.Servers[0].Tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", snapshot.Servers[0].Tools)
	}

	infoRaw, err := p.handleMCPInfo(context.Background(), &pluginsdk.Request{Payload: []byte(`{}`)})
	if err != nil {
		t.Fatalf("mcp info: %v", err)
	}
	var info providerInfo
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if len(info.Tools) != 1 || info.Tools[0].Name != "mcp_call" || info.Tools[0].Method != "call" {
		t.Fatalf("info = %+v", info)
	}

	payload, _ := json.Marshal(map[string]any{
		"arguments": map[string]any{
			"server":    "fake",
			"tool":      "echo",
			"arguments": map[string]string{"text": "hello"},
		},
	})
	raw, err := p.handleMCPCall(context.Background(), &pluginsdk.Request{Payload: payload})
	if err != nil {
		t.Fatalf("mcp call: %v", err)
	}
	var result providerToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.IsError || !strings.Contains(result.Content, "echo: hello") {
		t.Fatalf("result = %+v", result)
	}
	var meta map[string]any
	if err := json.Unmarshal(result.Meta, &meta); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta["server"] != "fake" || meta["tool"] != "echo" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestMCPConfigSaveRejectsUnknownType(t *testing.T) {
	p := &plugin{cfg: config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		MCPEnabled:     boolPtr(true),
		MCPTimeoutMS:   defaultMCPTimeoutMS,
	}, configPath: filepath.Join(t.TempDir(), "config.json")}
	payload, _ := json.Marshal(mcpConfigView{MCPServers: map[string]mcpEntry{
		"remote": {Type: "websocket", URL: "wss://example.com/mcp"},
	}})

	_, err := p.handleMCPConfigSave(context.Background(), &pluginsdk.Request{Payload: payload})
	if pluginsdk.Code(err) != codeMCPConfigInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestMCPManagerReportsUnsupportedTransport(t *testing.T) {
	p := &plugin{cfg: config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		MCPServers: map[string]mcpEntry{
			"remote": {Type: mcpTransportHTTP, URL: "https://example.com/mcp"},
		},
	}}
	p.startMCP()
	defer p.closeMCP()

	snapshot := p.mcp.snapshot()
	if len(snapshot.Servers) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	server := snapshot.Servers[0]
	if server.Status != mcpStatusError || !strings.Contains(server.LastError, "unsupported transport") {
		t.Fatalf("server = %+v", server)
	}
}

func TestLoadConfigUsesClaudeMCPServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
  "prompt_sections": [{"name": "identity", "order": -1000, "track": "static", "text": "identity"}],
  "mcpServers": {
    "rea": {
      "command": "npx",
      "args": ["-y", "rea-agents", "mcp"],
      "env": {"KEY": "value"}
    }
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	entry, ok := cfg.MCPServers["rea"]
	if !ok {
		t.Fatalf("servers = %+v", cfg.MCPServers)
	}
	if entry.Name != "rea" || entry.Type != mcpTransportStdio || entry.Command != "npx" || len(entry.Args) != 3 || entry.Env["KEY"] != "value" {
		t.Fatalf("entry = %+v", entry)
	}
	if !mcpGloballyEnabled(cfg) {
		t.Fatalf("mcp should default to enabled: %+v", cfg)
	}
}

func TestLoadConfigRejectsLegacyMCPServersArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
  "prompt_sections": [{"name": "identity", "order": -1000, "track": "static", "text": "identity"}],
  "mcp_servers": [{"name": "rea", "command": "npx"}]
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := loadConfig(path); err == nil {
		t.Fatal("legacy mcp_servers array accepted")
	}
}

func TestMCPManagerUnavailable(t *testing.T) {
	p := &plugin{}
	payload, _ := json.Marshal(map[string]any{
		"arguments": map[string]string{"server": "missing", "tool": "x"},
	})
	_, err := p.handleMCPCall(context.Background(), &pluginsdk.Request{Payload: payload})
	if pluginsdk.Code(err) != codeMCPUnavailable {
		t.Fatalf("err = %v", err)
	}
}
