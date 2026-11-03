package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	mcpStatusDisabled     = "disabled"
	mcpStatusUnconfigured = "unconfigured"
	mcpStatusConnecting   = "connecting"
	mcpStatusConnected    = "connected"
	mcpStatusError        = "error"

	maxMCPDescriptionBytes = 8 * 1024
)

type mcpServerStatus struct {
	Name        string
	Description string
	Transport   string
	Status      string
	Enabled     bool
	Runnable    bool
	Tools       []mcpToolInfo
	LastError   string
}

type mcpSnapshot struct {
	Enabled   bool
	TimeoutMS int
	Servers   []mcpServerStatus
}

type mcpServerState struct {
	entry      mcpEntry
	status     string
	tools      []mcpToolInfo
	lastError  string
	client     *mcpClient
	generation uint64
}

type mcpManager struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	servers    map[string]*mcpServerState
	order      []string
	generation uint64
	enabled    bool
	timeout    time.Duration
	logf       func(string, ...any)
}

type mcpConnectSpec struct {
	name       string
	entry      mcpEntry
	generation uint64
}

func newMCPManager(logf func(string, ...any)) *mcpManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &mcpManager{
		ctx:     ctx,
		cancel:  cancel,
		servers: make(map[string]*mcpServerState),
		timeout: defaultMCPTimeoutMS * time.Millisecond,
		logf:    logf,
	}
}

func (m *mcpManager) reload(cfg config) {
	timeout := cfg.MCPTimeoutMS
	if timeout <= 0 {
		timeout = defaultMCPTimeoutMS
	}

	m.mu.Lock()
	oldClients := make([]*mcpClient, 0, len(m.servers))
	for _, state := range m.servers {
		if state.client != nil {
			oldClients = append(oldClients, state.client)
		}
	}
	m.servers = make(map[string]*mcpServerState)
	m.order = nil
	m.generation++
	generation := m.generation
	m.enabled = cfg.MCPEnabled
	m.timeout = time.Duration(timeout) * time.Millisecond

	specs := make([]mcpConnectSpec, 0, len(cfg.MCPServers))
	for _, entry := range cfg.MCPServers {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		state := &mcpServerState{entry: entry, generation: generation}
		switch {
		case !mcpEnabled(entry) || !m.enabled:
			state.status = mcpStatusDisabled
		case strings.TrimSpace(entry.Command) == "":
			state.status = mcpStatusUnconfigured
		case entry.Transport != "" && entry.Transport != "stdio":
			state.status = mcpStatusError
			state.lastError = "unsupported transport: " + entry.Transport
		default:
			state.status = mcpStatusConnecting
			specs = append(specs, mcpConnectSpec{name: name, entry: entry, generation: generation})
		}
		m.servers[name] = state
		m.order = append(m.order, name)
	}
	m.mu.Unlock()

	for _, client := range oldClients {
		client.closeWithError(nil)
	}
	for _, spec := range specs {
		go m.connect(spec)
	}
}

func (m *mcpManager) connect(spec mcpConnectSpec) {
	if m.ctx.Err() != nil {
		return
	}
	client, err := newMCPClient(spec.name, spec.entry, m.ctx)
	if err != nil {
		m.setError(spec.name, spec.generation, err)
		return
	}
	client.onClose = func(closed *mcpClient) {
		m.handleClientClose(spec.name, spec.generation, closed)
	}

	timeout := m.currentTimeout(spec.entry)
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	defer cancel()
	if err := client.initialize(ctx); err != nil {
		client.closeWithError(err)
		m.setError(spec.name, spec.generation, err)
		return
	}
	tools, err := client.listTools(ctx)
	if err != nil {
		client.closeWithError(err)
		m.setError(spec.name, spec.generation, err)
		return
	}

	closeClient := false
	m.mu.Lock()
	state := m.servers[spec.name]
	if state == nil || state.generation != spec.generation || m.ctx.Err() != nil {
		closeClient = true
	} else {
		state.client = client
		state.status = mcpStatusConnected
		state.tools = tools
		state.lastError = ""
	}
	m.mu.Unlock()
	if closeClient {
		client.closeWithError(nil)
	}
}

func (m *mcpManager) currentTimeout(entry mcpEntry) time.Duration {
	if entry.TimeoutMS > 0 {
		return time.Duration(entry.TimeoutMS) * time.Millisecond
	}
	m.mu.Lock()
	timeout := m.timeout
	m.mu.Unlock()
	if timeout <= 0 {
		return defaultMCPTimeoutMS * time.Millisecond
	}
	return timeout
}

func (m *mcpManager) setError(name string, generation uint64, err error) {
	m.mu.Lock()
	state := m.servers[name]
	if state != nil && state.generation == generation {
		state.status = mcpStatusError
		state.lastError = err.Error()
	}
	m.mu.Unlock()
	if m.logf != nil {
		m.logf("mcp %s: %v", name, err)
	}
}

func (m *mcpManager) handleClientClose(name string, generation uint64, client *mcpClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.servers[name]
	if state == nil || state.generation != generation || state.client != client {
		return
	}
	state.client = nil
	state.status = mcpStatusError
	state.lastError = client.errorString()
	if state.lastError == "" {
		state.lastError = "connection closed"
	}
}

func (m *mcpManager) close() {
	m.mu.Lock()
	clients := make([]*mcpClient, 0, len(m.servers))
	for _, state := range m.servers {
		if state.client != nil {
			clients = append(clients, state.client)
		}
	}
	m.servers = make(map[string]*mcpServerState)
	m.order = nil
	m.mu.Unlock()

	for _, client := range clients {
		client.closeWithError(nil)
	}
	m.cancel()
}

func (m *mcpManager) snapshot() mcpSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	servers := make([]mcpServerStatus, 0, len(m.order))
	for _, name := range m.order {
		state := m.servers[name]
		if state == nil {
			continue
		}
		tools := append([]mcpToolInfo(nil), state.tools...)
		enabled := m.enabled && mcpEnabled(state.entry)
		servers = append(servers, mcpServerStatus{
			Name:        state.entry.Name,
			Description: state.entry.Description,
			Transport:   state.entry.Transport,
			Status:      state.status,
			Enabled:     enabled,
			Runnable:    enabled && strings.TrimSpace(state.entry.Command) != "",
			Tools:       tools,
			LastError:   state.lastError,
		})
	}
	return mcpSnapshot{
		Enabled:   m.enabled,
		TimeoutMS: int(m.timeout / time.Millisecond),
		Servers:   servers,
	}
}

func (m *mcpManager) definitions() []providerTool {
	snapshot := m.snapshot()
	if !snapshot.Enabled {
		return nil
	}

	lines := make([]string, 0)
	for _, server := range snapshot.Servers {
		if !server.Runnable {
			continue
		}
		line := "- " + server.Name
		if server.Description != "" {
			line += ": " + server.Description
		}
		if len(server.Tools) > 0 {
			names := make([]string, 0, len(server.Tools))
			for _, tool := range server.Tools {
				names = append(names, tool.Name)
			}
			line += " (tools: " + strings.Join(names, ", ") + ")"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}

	description := "Call a tool exposed by an MCP server. Use the server and tool names listed in the runtime context.\n\n" +
		strings.Join(lines, "\n")
	description = truncateSkillText(description, maxMCPDescriptionBytes)
	return []providerTool{{
		Name:        "mcp_call",
		Description: description,
		Parameters:  mcpCallParameters,
		Method:      "call",
	}}
}

func (m *mcpManager) runtimeEntries() []mcpEntry {
	snapshot := m.snapshot()
	if !snapshot.Enabled {
		return nil
	}
	out := make([]mcpEntry, 0, len(snapshot.Servers))
	for _, server := range snapshot.Servers {
		if !server.Enabled || server.Status != mcpStatusConnected {
			continue
		}
		tools := make([]string, 0, len(server.Tools))
		for _, tool := range server.Tools {
			tools = append(tools, tool.Name)
		}
		out = append(out, mcpEntry{
			Name:        server.Name,
			Description: server.Description,
			Tools:       tools,
		})
	}
	return out
}

func (m *mcpManager) call(ctx context.Context, server, tool string, arguments json.RawMessage) (mcpToolCallResult, error) {
	m.mu.Lock()
	state := m.servers[server]
	if state == nil || state.status != mcpStatusConnected || state.client == nil {
		status := ""
		if state != nil {
			status = state.status
		}
		m.mu.Unlock()
		return mcpToolCallResult{}, fmt.Errorf("mcp server %s is not connected (%s)", server, status)
	}
	client := state.client
	entry := state.entry
	m.mu.Unlock()

	timeout := m.currentTimeout(entry)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.callTool(callCtx, tool, arguments)
}

func (m *mcpManager) resourceView() resourceMCPView {
	snapshot := m.snapshot()
	servers := make([]resourceMCPServer, 0, len(snapshot.Servers))
	for _, server := range snapshot.Servers {
		tools := make([]resourceMCPTool, 0, len(server.Tools))
		for _, tool := range server.Tools {
			tools = append(tools, resourceMCPTool{Name: tool.Name, Description: tool.Description})
		}
		servers = append(servers, resourceMCPServer{
			Name:        server.Name,
			Description: server.Description,
			Transport:   server.Transport,
			Status:      server.Status,
			Enabled:     server.Enabled,
			Tools:       tools,
			LastError:   server.LastError,
		})
	}
	return resourceMCPView{
		Enabled:   snapshot.Enabled,
		TimeoutMS: snapshot.TimeoutMS,
		Servers:   servers,
	}
}
