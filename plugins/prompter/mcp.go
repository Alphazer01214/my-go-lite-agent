package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const maxMCPToolContentBytes = 1 << 20

var mcpCallParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "server": {"type": "string", "description": "MCP server name."},
    "tool": {"type": "string", "description": "Tool name exposed by that server."},
    "arguments": {"type": "object", "description": "Arguments object for the MCP tool."}
  },
  "required": ["server", "tool"],
  "additionalProperties": false
}`)

type mcpCallArgs struct {
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type mcpConfigView struct {
	MCPServers map[string]mcpEntry `json:"mcpServers"`
}

func mcpEnabled(entry mcpEntry) bool {
	return entry.Enabled == nil || *entry.Enabled
}

func (p *plugin) startMCP() {
	if p.mcp != nil {
		return
	}
	p.mcp = newMCPManager(func(format string, args ...any) {
		logf("mcp: "+format, args...)
	})
	p.mcp.reload(p.currentConfig())
}

func (p *plugin) closeMCP() {
	if p.mcp != nil {
		p.mcp.close()
	}
}

func (p *plugin) reloadMCP() {
	if p.mcp != nil {
		p.mcp.reload(p.currentConfig())
	}
}

func (p *plugin) runtimeMCPServers(cfg config) map[string]mcpEntry {
	if p.mcp == nil {
		return cfg.MCPServers
	}
	entries := p.mcp.runtimeEntries()
	out := make(map[string]mcpEntry, len(entries))
	for _, entry := range entries {
		out[entry.Name] = entry
	}
	return out
}

func (p *plugin) resourceMCP(cfg config) resourceMCPView {
	if p.mcp != nil {
		return p.mcp.resourceView()
	}
	servers := make([]resourceMCPServer, 0, len(cfg.MCPServers))
	for _, name := range sortedMCPNames(cfg.MCPServers) {
		entry := cfg.MCPServers[name]
		status := "configured"
		if !mcpEnabled(entry) || !mcpGloballyEnabled(cfg) {
			status = mcpStatusDisabled
		} else if strings.TrimSpace(entry.Command) == "" && entry.Type == mcpTransportStdio {
			status = mcpStatusUnconfigured
		}
		transport := entry.Type
		if transport == "" {
			transport = mcpTransportStdio
		}
		tools := make([]resourceMCPTool, 0, len(entry.Tools))
		for _, tool := range entry.Tools {
			tools = append(tools, resourceMCPTool{Name: tool})
		}
		servers = append(servers, resourceMCPServer{
			Name:        name,
			Description: entry.Description,
			Transport:   transport,
			Status:      status,
			Enabled:     mcpEnabled(entry) && mcpGloballyEnabled(cfg),
			Tools:       tools,
		})
	}
	return resourceMCPView{
		Enabled:   mcpGloballyEnabled(cfg),
		TimeoutMS: cfg.MCPTimeoutMS,
		Servers:   servers,
	}
}

func (p *plugin) handleMCPInfo(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	tools := []providerTool(nil)
	if p.mcp != nil {
		tools = p.mcp.definitions()
	}
	return json.Marshal(providerInfo{Capability: "mcp", Tools: tools})
}

func (p *plugin) handleMCPCall(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args mcpCallArgs
	if err := decodeProviderArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	server := strings.TrimSpace(args.Server)
	tool := strings.TrimSpace(args.Tool)
	if server == "" || tool == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "server and tool are required")
	}
	arguments := args.Arguments
	if len(arguments) == 0 || strings.TrimSpace(string(arguments)) == "null" {
		arguments = json.RawMessage(`{}`)
	} else {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(arguments, &object); err != nil || object == nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "arguments must be a JSON object")
		}
	}
	if p.mcp == nil {
		return nil, pluginsdk.ErrCode(codeMCPUnavailable, "mcp manager is not running")
	}

	started := time.Now()
	result, err := p.mcp.call(ctx, server, tool, arguments)
	if err != nil {
		return nil, pluginsdk.ErrCode(codeMCPCallFailed, err.Error())
	}
	content := mcpToolContent(result)
	truncated := false
	if len(content) > maxMCPToolContentBytes {
		content = truncateSkillText(content, maxMCPToolContentBytes)
		truncated = true
	}
	toolResult := providerToolSuccess(content, map[string]any{
		"server":      server,
		"tool":        tool,
		"duration_ms": time.Since(started).Milliseconds(),
	})
	toolResult.IsError = result.IsError
	toolResult.Truncated = truncated
	return json.Marshal(toolResult)
}

func (p *plugin) handleMCPConfigGet(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	cfg := p.currentConfig()
	servers := cfg.MCPServers
	if servers == nil {
		servers = map[string]mcpEntry{}
	}
	return json.Marshal(mcpConfigView{MCPServers: servers})
}

func (p *plugin) handleMCPConfigSave(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	var input mcpConfigView
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}

	cfg := p.currentConfig()
	cfg.MCPServers = input.MCPServers
	if cfg.MCPServers == nil {
		cfg.MCPServers = map[string]mcpEntry{}
	}
	if err := cfg.normalize(); err != nil {
		return nil, pluginsdk.ErrCode(codeMCPConfigInvalid, err.Error())
	}

	path := p.configPath
	if path == "" {
		path = configPath
	}
	if err := saveConfig(path, cfg); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, fmt.Sprintf("save config: %v", err))
	}

	p.configMu.Lock()
	p.cfg = cfg
	p.configMu.Unlock()
	p.reloadMCP()

	return json.Marshal(settingsSaveResp{Saved: true})
}
