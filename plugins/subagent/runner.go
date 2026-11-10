package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	sdk      sdkCaller
	configMu sync.RWMutex
	cfg      config
}

func newPlugin(sdk sdkCaller, cfg config) *plugin {
	return &plugin{sdk: sdk, cfg: cfg}
}

func (p *plugin) currentConfig() config {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.cfg
}

func (p *plugin) setConfig(cfg config) {
	p.configMu.Lock()
	p.cfg = cfg
	p.configMu.Unlock()
}

func (p *plugin) call(
	ctx context.Context,
	contextID string,
	capability, method string,
	payload any,
) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	if contextID != "" {
		return p.sdk.CallInContext(ctx, contextID, capability, method, body)
	}
	return p.sdk.Call(ctx, capability, method, body)
}

func (p *plugin) run(
	ctx context.Context,
	contextID string,
	input providerInput,
	in runIn,
) (toolResult, error) {
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return toolResult{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "prompt is required")
	}
	parentID := strings.TrimSpace(input.SessionID)
	if parentID == "" {
		return toolResult{}, pluginsdk.ErrCode(codeParentUnresolved, "parent session_id is required")
	}

	parent, err := p.sessionInfo(ctx, contextID, parentID)
	if err != nil {
		return toolResult{}, pluginsdk.ErrCode(codeParentUnresolved, "parent session: "+err.Error())
	}
	workspace := strings.TrimSpace(parent.Workspace)
	if workspace == "" {
		workspace = strings.TrimSpace(input.Workspace)
	}

	depth, err := p.parentDepth(ctx, contextID, parentID)
	if err != nil {
		return toolResult{}, pluginsdk.ErrCode(codeParentUnresolved, "parent chain: "+err.Error())
	}
	maxDepth := p.currentConfig().MaxDepth
	if depth > maxDepth {
		return toolResult{}, pluginsdk.ErrCode(codeDepthExceeded,
			fmt.Sprintf("delegation depth %d exceeds max_depth %d", depth, maxDepth))
	}

	started := time.Now()
	child, err := p.createChildSession(ctx, contextID, sessionCreateReq{
		Title:           "subagent: " + shortLabel(prompt, 48),
		Workspace:       workspace,
		ParentSessionID: parentID,
		Origin:          "subagent",
		Model:           parent.Model,
	})
	if err != nil {
		return toolResult{}, pluginsdk.ErrCode(codeSpawnFailed, err.Error())
	}

	p.appendFactsBestEffort(ctx, parentID, []sessionFact{{
		Type: factTypeSpawned,
		Role: "host",
		Meta: mustJSON(map[string]any{
			"child_session_id":  child.SessionId,
			"state":             "running",
			"parent_session_id": parentID,
			"delegation_depth":  depth,
			"started_ts":        started.UnixMilli(),
		}),
	}})
	p.publishRunsState(parentID, "running")

	turn, turnErr := p.callTurn(ctx, contextID, child.SessionId, workspace, parent.Model, prompt)
	state := "completed"
	content := ""
	isError := false
	errText := ""
	switch {
	case turnErr != nil:
		if code := canceledCode(ctx, turnErr); code != "" {
			p.finishParent(ctx, parentID, child.SessionId, depth, "cancelled", time.Since(started), 0, turnErr.Error())
			return toolResult{}, pluginsdk.ErrCode(code, "subagent cancelled")
		}
		state = "failed"
		isError = true
		errText = turnErr.Error()
		content = "Subagent failed: " + errText
	default:
		content = finalReply(turn.Messages)
		if strings.TrimSpace(content) == "" {
			state = "failed"
			isError = true
			content = "Subagent returned no text."
		}
	}

	turnCount := countAssistantMessages(turn.Messages)
	duration := time.Since(started)
	content, truncated := truncateUTF8(content, maxReplyBytes)
	if strings.TrimSpace(content) == "" {
		content = "(no output)"
	}
	p.finishParent(ctx, parentID, child.SessionId, depth, state, duration, turnCount, errText)

	meta := map[string]any{
		"child_session_id": child.SessionId,
		"state":            state,
		"duration_ms":      duration.Milliseconds(),
		"turn_count":       turnCount,
	}
	if errText != "" {
		meta["error"] = errText
	}
	return toolResult{
		Content:   content,
		IsError:   isError,
		Truncated: truncated,
		Meta:      meta,
	}, nil
}

func (p *plugin) finishParent(
	ctx context.Context,
	parentID, childID string,
	depth int,
	state string,
	duration time.Duration,
	turnCount int,
	errText string,
) {
	meta := map[string]any{
		"child_session_id":  childID,
		"state":             state,
		"parent_session_id": parentID,
		"delegation_depth":  depth,
		"duration_ms":       duration.Milliseconds(),
		"turn_count":        turnCount,
	}
	if errText != "" {
		meta["error"] = errText
	}
	p.appendFactsBestEffort(ctx, parentID, []sessionFact{{
		Type: factTypeFinished,
		Role: "host",
		Meta: mustJSON(meta),
	}})
	p.publishRunsState(parentID, state)
}

func (p *plugin) parentDepth(ctx context.Context, contextID, parentID string) (int, error) {
	depth := 0
	id := parentID
	seen := make(map[string]bool, 4)
	for id != "" {
		if seen[id] {
			return 0, fmt.Errorf("session parent cycle at %s", id)
		}
		seen[id] = true
		info, err := p.sessionInfo(ctx, contextID, id)
		if err != nil {
			return 0, err
		}
		depth++
		if depth > maxAllowedDepth {
			return depth, nil
		}
		id = info.ParentSessionId
	}
	return depth, nil
}

func (p *plugin) sessionInfo(ctx context.Context, contextID, sessionID string) (sessionInfo, error) {
	raw, err := p.call(ctx, contextID, "session", "info", map[string]string{"session_id": sessionID})
	if err != nil {
		return sessionInfo{}, err
	}
	var info sessionInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return sessionInfo{}, fmt.Errorf("bad session.info response: %w", err)
	}
	return info, nil
}

func (p *plugin) createChildSession(
	ctx context.Context,
	contextID string,
	req sessionCreateReq,
) (sessionInfo, error) {
	raw, err := p.call(ctx, contextID, "session", "create", req)
	if err != nil {
		return sessionInfo{}, err
	}
	var meta sessionInfo
	if err := json.Unmarshal(raw, &meta); err != nil {
		return sessionInfo{}, fmt.Errorf("bad session.create response: %w", err)
	}
	if strings.TrimSpace(meta.SessionId) == "" {
		return sessionInfo{}, fmt.Errorf("session.create returned no session_id")
	}
	return meta, nil
}

func (p *plugin) callTurn(
	ctx context.Context,
	contextID, sessionID, workspace, model, prompt string,
) (turnResp, error) {
	raw, err := p.call(ctx, contextID, "loop", "turn", turnReq{
		Workspace: workspace,
		SessionID: sessionID,
		UserInput: prompt,
		Model:     model,
	})
	if err != nil {
		return turnResp{}, err
	}
	var out turnResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return turnResp{}, fmt.Errorf("bad loop.turn response: %w", err)
	}
	return out, nil
}

func (p *plugin) appendFacts(
	ctx context.Context,
	contextID, sessionID string,
	facts []sessionFact,
) (appendResp, error) {
	raw, err := p.call(ctx, contextID, "session", "append", appendReq{
		SessionID: sessionID,
		Facts:     facts,
	})
	if err != nil {
		return appendResp{}, err
	}
	var out appendResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return appendResp{}, err
	}
	return out, nil
}

// Parent facts are observational: a failure must never fail the delegated task.
func (p *plugin) appendFactsBestEffort(ctx context.Context, sessionID string, facts []sessionFact) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if _, err := p.appendFacts(writeCtx, "", sessionID, facts); err != nil {
		logf("append %s: %v", sessionID, err)
	}
}

func finalReply(messages []pluginsdk.ChatMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role != pluginsdk.RoleAssistant {
			continue
		}
		if text := strings.TrimSpace(message.Text); text != "" {
			return text
		}
	}
	return ""
}

func countAssistantMessages(messages []pluginsdk.ChatMessage) int {
	count := 0
	for _, message := range messages {
		if message.Role == pluginsdk.RoleAssistant {
			count++
		}
	}
	return count
}

func canceledCode(ctx context.Context, err error) string {
	if err == nil {
		return ""
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return pluginsdk.CodeCallDeadlineExceeded
		}
		return pluginsdk.CodeCallCancelled
	}
	if errors.Is(err, context.Canceled) {
		return pluginsdk.CodeCallCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return pluginsdk.CodeCallDeadlineExceeded
	}
	switch pluginsdk.Code(err) {
	case pluginsdk.CodeCallCancelled, pluginsdk.CodeCallDeadlineExceeded:
		return pluginsdk.Code(err)
	}
	return ""
}

func shortLabel(text string, limit int) string {
	compact := strings.Join(strings.Fields(text), " ")
	runes := []rune(compact)
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func mustJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return body
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "subagent: "+format+"\n", args...)
}
