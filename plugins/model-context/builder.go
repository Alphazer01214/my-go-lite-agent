package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk       sdkCaller
	cfg       config
	mu        sync.Mutex
	snapshots map[string]snapshot
	revision  int
	seq       atomic.Int64
}

func newPlugin(sdk sdkCaller) *plugin {
	return &plugin{sdk: sdk, cfg: loadConfig(), snapshots: make(map[string]snapshot)}
}

func loadConfig() config {
	cfg := defaultConfig()
	data, err := os.ReadFile("config.json")
	if err != nil {
		return cfg
	}
	var raw config
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}
	if strings.TrimSpace(raw.SystemPrompt) != "" {
		cfg.SystemPrompt = raw.SystemPrompt
	}
	if raw.MaxContextChars > 0 {
		cfg.MaxContextChars = raw.MaxContextChars
	}
	return cfg
}

func (p *plugin) build(ctx context.Context, contextID string, in buildIn) (buildOut, error) {
	if in.SessionID == "" {
		return buildOut{}, pluginsdk.ErrCode("bad_arguments", "session_id is required")
	}
	messages, err := p.sessionMessages(ctx, contextID, in.SessionID)
	if err != nil {
		return buildOut{}, err
	}
	tools, err := p.toolDefinitions(ctx, contextID, in)
	if err != nil {
		return buildOut{}, err
	}
	parts := []snapshotPart{}
	warnings := []string{}
	systemPrompt := strings.TrimSpace(p.cfg.SystemPrompt)
	if extra := workspaceInstructions(in.Workspace); extra != "" {
		systemPrompt += "\n\nWorkspace instructions:\n" + extra
		parts = append(parts, snapshotPart{Source: "workspace/AGENTS.md", Bytes: len(extra)})
	}
	conversation := make([]chatMessage, 0, len(messages)+1)
	for _, message := range messages {
		if message.Role == "system" {
			if strings.TrimSpace(message.Content) != "" {
				systemPrompt += "\n\nSession instructions:\n" + message.Content
			}
			continue
		}
		conversation = append(conversation, message)
	}
	if systemPrompt != "" {
		conversation = append([]chatMessage{{Role: "system", Content: systemPrompt}}, conversation...)
		parts = append(parts, snapshotPart{Source: "system_prompt", Bytes: len(systemPrompt)})
	}
	parts = append(parts, snapshotPart{Source: "session", Bytes: messageBytes(conversation)})
	if len(tools) > 0 {
		body, _ := json.Marshal(tools)
		parts = append(parts, snapshotPart{Source: "tools", Bytes: len(body)})
	}
	maxChars := p.cfg.MaxContextChars
	if in.MaxChars != nil && *in.MaxChars > 0 {
		maxChars = *in.MaxChars
	}
	conversation, trimmed := trimMessages(conversation, maxChars)
	if trimmed > 0 {
		warnings = append(warnings, fmt.Sprintf("trimmed %d oldest context messages", trimmed))
	}
	body, err := json.Marshal(struct {
		Messages []chatMessage    `json:"messages"`
		Tools    []toolDefinition `json:"tools"`
	}{Messages: conversation, Tools: tools})
	if err != nil {
		return buildOut{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	sum := sha256.Sum256(body)
	revision := p.nextRevision()
	id := fmt.Sprintf("mc-%d-%d", time.Now().UnixNano(), p.seq.Add(1))
	snap := snapshot{
		ModelContextID: id, SessionID: in.SessionID, TurnID: in.TurnID, StepID: in.StepID,
		Revision: revision, Messages: conversation, Tools: tools, Parts: parts,
		TokenEstimate: estimateTokens(conversation, tools), Hash: hex.EncodeToString(sum[:]),
		Warnings: warnings,
	}
	p.mu.Lock()
	p.snapshots[id] = snap
	p.mu.Unlock()
	if err := p.persistSnapshot(ctx, contextID, snap); err != nil {
		return buildOut{}, err
	}
	return buildOut{
		ModelContextID: id, Messages: conversation, Tools: tools,
		TokenEstimate: snap.TokenEstimate, Warnings: warnings,
	}, nil
}

func (p *plugin) inspect(in inspectIn) (snapshot, error) {
	if in.ModelContextID == "" {
		return snapshot{}, pluginsdk.ErrCode("bad_arguments", "model_context_id is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	snap, ok := p.snapshots[in.ModelContextID]
	if !ok {
		return snapshot{}, pluginsdk.ErrCode("model_context_not_found", "snapshot is not in the current process")
	}
	return snap, nil
}

func (p *plugin) nextRevision() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.revision++
	return p.revision
}

func (p *plugin) sessionMessages(ctx context.Context, contextID, sessionID string) ([]chatMessage, error) {
	payload, _ := json.Marshal(deriveIn{SessionID: sessionID})
	raw, err := p.call(ctx, contextID, "session", "derive", payload)
	if err != nil {
		return nil, err
	}
	var out deriveOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "decode session.derive: "+err.Error())
	}
	return out.Messages, nil
}

func (p *plugin) toolDefinitions(ctx context.Context, contextID string, in buildIn) ([]toolDefinition, error) {
	if in.Scheme == "chat" && len(in.AllowedTools) == 0 {
		return nil, nil
	}
	raw, err := p.call(ctx, contextID, "tools", "list", json.RawMessage(`{}`))
	if err != nil {
		return nil, nil
	}
	var listed toolsListOut
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, nil
	}
	allowed := stringSet(in.AllowedTools)
	denied := stringSet(in.DeniedTools)
	out := make([]toolDefinition, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		if len(allowed) > 0 && !allowed[tool.Name] {
			continue
		}
		if denied[tool.Name] {
			continue
		}
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		description := tool.Description
		if tool.Risk != "" {
			description += "\nRisk: " + tool.Risk + "."
		}
		out = append(out, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name: tool.Name, Description: description, Parameters: schema,
			},
		})
	}
	return out, nil
}

func (p *plugin) persistSnapshot(ctx context.Context, contextID string, snap snapshot) error {
	meta, err := json.Marshal(snap)
	if err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	payload, err := json.Marshal(appendIn{
		SessionID: snap.SessionID,
		Facts: []factIn{{
			Type: "context_snapshot", Role: "host", Meta: meta,
		}},
	})
	if err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	_, err = p.call(ctx, contextID, "session", "append", payload)
	return err
}

func (p *plugin) call(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if contextID != "" {
		return p.sdk.CallInContext(ctx, contextID, capability, method, payload)
	}
	return p.sdk.Call(ctx, capability, method, payload)
}

func workspaceInstructions(workspace string) string {
	if workspace == "" || !filepath.IsAbs(workspace) {
		return ""
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return ""
	}
	defer root.Close()
	data, err := root.ReadFile("AGENTS.md")
	if err != nil {
		return ""
	}
	if len(data) > 32<<10 {
		data = data[:32<<10]
		for !utf8.Valid(data) {
			data = data[:len(data)-1]
		}
	}
	return string(data)
}

func trimMessages(messages []chatMessage, maxChars int) ([]chatMessage, int) {
	if maxChars <= 0 || messageChars(messages) <= maxChars {
		return messages, 0
	}
	trimmed := 0
	for len(messages) > 1 && messageChars(messages) > maxChars {
		index := 1
		if messages[0].Role != "system" {
			index = 0
		}
		if index >= len(messages) {
			break
		}
		messages = append(messages[:index], messages[index+1:]...)
		trimmed++
		for len(messages) > 1 && messages[0].Role == "tool" {
			messages = messages[1:]
			trimmed++
		}
	}
	return messages, trimmed
}

func messageChars(messages []chatMessage) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total
}

func messageBytes(messages []chatMessage) int {
	body, _ := json.Marshal(messages)
	return len(body)
}

func estimateTokens(messages []chatMessage, tools []toolDefinition) int {
	body, _ := json.Marshal(struct {
		Messages []chatMessage    `json:"messages"`
		Tools    []toolDefinition `json:"tools"`
	}{messages, tools})
	return (len(body) + 3) / 4
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
