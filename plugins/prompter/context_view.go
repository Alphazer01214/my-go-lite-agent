package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	maxCachedContexts      = 8
	maxContextMessageBytes = 16 * 1024
	maxContextTotalBytes   = 1 << 20
)

type contextGetReq struct {
	SessionID string `json:"session_id,omitempty"`
}

type contextMessageView struct {
	Seq        int                  `json:"seq,omitempty"`
	Role       string               `json:"role"`
	Tokens     int                  `json:"tokens"`
	Text       string               `json:"text"`
	Reasoning  string               `json:"reasoning,omitempty"`
	Truncated  bool                 `json:"truncated,omitempty"`
	Files      []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls  []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

type contextToolView struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type contextUsage struct {
	TokenEstimate         int     `json:"token_estimate"`
	ContextWindow         int     `json:"context_window,omitempty"`
	Percent               float64 `json:"percent,omitempty"`
	MessageCount          int     `json:"message_count"`
	ToolCount             int     `json:"tool_count"`
	ToolCatalogGeneration int64   `json:"tool_catalog_generation,omitempty"`
}

type contextView struct {
	SessionID   string               `json:"session_id,omitempty"`
	TurnID      string               `json:"turn_id,omitempty"`
	Model       string               `json:"model,omitempty"`
	Source      string               `json:"source"`
	AssembledAt string               `json:"assembled_at"`
	Usage       contextUsage         `json:"usage"`
	Messages    []contextMessageView `json:"messages"`
	Tools       []contextToolView    `json:"tools"`
	Snapshot    *snapshotMeta        `json:"snapshot,omitempty"`
	Truncated   bool                 `json:"truncated,omitempty"`
}

type contextCacheEntry struct {
	view      contextView
	updatedAt time.Time
	seq       uint64
}

type contextState struct {
	SessionID string       `json:"session_id,omitempty"`
	TurnID    string       `json:"turn_id,omitempty"`
	Model     string       `json:"model,omitempty"`
	Usage     contextUsage `json:"usage"`
	UpdatedAt string       `json:"updated_at"`
}

func (p *plugin) handleContextGet(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var input contextGetReq
	if req != nil && len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &input); err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
	}
	view, err := p.contextViewFor(ctx, strings.TrimSpace(input.SessionID))
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

func (p *plugin) contextViewFor(ctx context.Context, sessionID string) (contextView, error) {
	if sessionID == "" {
		p.contextMu.Lock()
		sessionID = p.lastContext
		p.contextMu.Unlock()
		if sessionID == "" {
			return emptyContextView(), nil
		}
	}

	p.contextMu.Lock()
	entry, ok := p.contexts[sessionID]
	p.contextMu.Unlock()
	if ok {
		view := entry.view
		p.enrichContextUsage(ctx, &view)
		return view, nil
	}

	return p.deriveContextView(ctx, sessionID)
}

func (p *plugin) deriveContextView(ctx context.Context, sessionID string) (contextView, error) {
	info, err := p.sessionInfo(ctx, sessionID)
	if err != nil {
		return contextView{}, err
	}
	facts, err := p.sessionFacts(ctx, sessionID)
	if err != nil {
		return contextView{}, err
	}

	model := strings.TrimSpace(info.Model)
	baseline, err := p.baselineFacts(info.Workspace, model)
	if err != nil {
		return contextView{}, err
	}

	snapshotBaseline := baseline
	system := latestFact(facts, factTypeSystemPrompt)
	runtime := latestFact(facts, factTypeRuntimeContext)
	if system != nil && runtime != nil {
		snapshotBaseline = []factIn{
			{Type: factTypeSystemPrompt, TextContent: system.TextContent},
			{Type: factTypeRuntimeContext, TextContent: runtime.TextContent},
		}
	} else {
		for _, fact := range baseline {
			facts = append(facts, sessionFact{
				Type:        fact.Type,
				Role:        fact.Role,
				TextContent: fact.TextContent,
				Meta:        fact.Meta,
			})
		}
	}

	tools, err := p.modelTools(ctx)
	if err != nil {
		return contextView{}, err
	}
	messages := projectMessages(facts)
	snapshot := buildSnapshotMeta(sessionID, model, snapshotBaseline, messages, tools)
	view := buildContextView("derived", sessionID, "", model, messages, tools, snapshot)
	p.enrichContextUsage(ctx, &view)
	return view, nil
}

func (p *plugin) rememberContext(
	sessionID, turnID, model string,
	messages []pluginsdk.ChatMessage,
	tools toolCatalog,
	snapshot snapshotMeta,
) {
	view := buildContextView("live", sessionID, turnID, model, messages, tools, snapshot)

	p.contextMu.Lock()
	if p.contexts == nil {
		p.contexts = make(map[string]contextCacheEntry)
	}
	p.contextSeq++
	p.contexts[sessionID] = contextCacheEntry{view: view, updatedAt: time.Now(), seq: p.contextSeq}
	p.lastContext = sessionID
	for len(p.contexts) > maxCachedContexts {
		oldestKey := ""
		var oldestSeq uint64
		for key, entry := range p.contexts {
			if key == sessionID {
				continue
			}
			if oldestKey == "" || entry.seq < oldestSeq {
				oldestKey = key
				oldestSeq = entry.seq
			}
		}
		if oldestKey == "" {
			break
		}
		delete(p.contexts, oldestKey)
	}
	p.contextMu.Unlock()

	p.publishContextState(view)
}

func (p *plugin) publishContextState(view contextView) {
	if p.sdk == nil {
		return
	}
	state, err := json.Marshal(contextState{
		SessionID: view.SessionID,
		TurnID:    view.TurnID,
		Model:     view.Model,
		Usage:     view.Usage,
		UpdatedAt: view.AssembledAt,
	})
	if err != nil {
		return
	}
	go func() {
		_ = p.sdk.WebUI().SetState("context", state)
	}()
}

func buildContextView(
	source, sessionID, turnID, model string,
	messages []pluginsdk.ChatMessage,
	tools toolCatalog,
	snapshot snapshotMeta,
) contextView {
	view := contextView{
		SessionID:   sessionID,
		TurnID:      turnID,
		Model:       model,
		Source:      source,
		AssembledAt: time.Now().UTC().Format(time.RFC3339Nano),
		Usage: contextUsage{
			TokenEstimate:         estimateTokens(messages),
			MessageCount:          len(messages),
			ToolCount:             len(tools.Tools),
			ToolCatalogGeneration: tools.Generation,
		},
		Messages: make([]contextMessageView, 0, len(messages)),
		Tools:    make([]contextToolView, 0, len(tools.Tools)),
		Snapshot: &snapshot,
	}

	remaining := maxContextTotalBytes
	for _, message := range messages {
		text, truncated := truncateContextText(message.Text, maxContextMessageBytes, remaining)
		remaining -= len(text)
		if remaining < 0 {
			remaining = 0
		}
		view.Truncated = view.Truncated || truncated
		view.Messages = append(view.Messages, contextMessageView{
			Seq:        message.Seq,
			Role:       message.Role,
			Tokens:     estimateMessageTokens(message.Text),
			Text:       text,
			Reasoning:  message.Reasoning,
			Truncated:  truncated,
			Files:      message.Files,
			ToolCalls:  message.ToolCalls,
			ToolCallID: message.ToolCallID,
		})
	}
	for _, tool := range tools.Tools {
		view.Tools = append(view.Tools, contextToolView{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	return view
}

func emptyContextView() contextView {
	return contextView{
		Source:      "none",
		AssembledAt: time.Now().UTC().Format(time.RFC3339Nano),
		Messages:    []contextMessageView{},
		Tools:       []contextToolView{},
	}
}

func (p *plugin) enrichContextUsage(ctx context.Context, view *contextView) {
	if view == nil || view.Model == "" {
		return
	}
	window := p.modelContextWindow(ctx, view.Model)
	if window <= 0 {
		return
	}
	view.Usage.ContextWindow = window
	percent := float64(view.Usage.TokenEstimate) * 100 / float64(window)
	view.Usage.Percent = math.Round(percent*10) / 10
}

func (p *plugin) modelContextWindow(ctx context.Context, model string) int {
	if p.sdk == nil || model == "" {
		return 0
	}
	raw, err := p.sdk.Call(ctx, "llm", "models.list", json.RawMessage(`{}`))
	if err != nil {
		return 0
	}
	var out struct {
		Models []struct {
			ID            string `json:"id"`
			ContextWindow int    `json:"context_window"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0
	}
	for _, item := range out.Models {
		if item.ID == model {
			return item.ContextWindow
		}
	}
	return 0
}

func estimateMessageTokens(text string) int {
	return (len([]rune(text))+3)/4 + 4
}

func truncateContextText(value string, perLimit, remaining int) (string, bool) {
	limit := perLimit
	if remaining < limit {
		limit = remaining
	}
	if len(value) <= limit {
		return value, false
	}
	if limit <= 0 {
		return "", true
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func checkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "call deadline exceeded")
		}
		return pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "call cancelled")
	}
	return nil
}
