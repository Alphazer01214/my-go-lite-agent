package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const noToolsSentinel = "__subagent_no_tools__"

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	sdk sdkCaller
	cfg config
}

func newPlugin(sdk sdkCaller) *plugin {
	return &plugin{sdk: sdk, cfg: loadConfig()}
}

// run is the whole subagent lifecycle: create a child session, run one turn on
// the caller's context, then return the child's final answer as the tool
// result. Cancellation of the parent turn cancels the child turn with it.
func (p *plugin) run(ctx context.Context, input providerInput, in runIn) (toolResult, error) {
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return toolResult{}, errBadArguments("prompt is required")
	}
	parentID := input.SessionID
	if parentID == "" {
		return toolResult{}, errCoded(codeParentUnresolved, "parent session_id is required")
	}
	parent, err := p.sessionInfo(ctx, parentID)
	if err != nil {
		return toolResult{}, errCoded(codeParentUnresolved, "parent session: "+err.Error())
	}
	if parent.Workspace == "" {
		return toolResult{}, errCoded(codeParentUnresolved, "parent session has no workspace")
	}
	depth := parent.DelegationDepth + 1
	if depth > p.cfg.MaxDepth {
		return toolResult{}, errCoded(codeDepthExceeded,
			fmt.Sprintf("delegation depth %d exceeds max_depth %d", depth, p.cfg.MaxDepth))
	}
	allowed, err := p.childTools(ctx)
	if err != nil {
		return toolResult{}, err
	}
	maxSteps := in.MaxSteps
	if maxSteps <= 0 {
		maxSteps = p.cfg.DefaultMaxSteps
	}

	childID, err := p.createChildSession(ctx, parent, depth, prompt)
	if err != nil {
		return toolResult{}, errCoded(codeSpawnFailed, err.Error())
	}
	started := time.Now()
	if _, err := p.appendFacts(ctx, parentID, []factIn{{
		Type: "subagent_spawned", Role: "host",
		Meta: mustJSON(map[string]any{
			"child_session_id": childID, "state": "running",
			"parent_session_id": parentID, "delegation_depth": depth,
			"started_ts": started.UnixMilli(),
		}),
	}}); err != nil {
		return toolResult{}, errCoded(codeSpawnFailed, "write spawn fact: "+err.Error())
	}

	out, turnErr := p.callTurn(ctx, childID, parent, depth, prompt, allowed, maxSteps)
	state := "completed"
	content := out.Reply
	isError := false
	errText := ""
	switch {
	case turnErr != nil:
		state = "failed"
		isError = true
		errText = turnErr.Error()
		if ctx.Err() == context.Canceled {
			state = "cancelled"
		}
		if ctx.Err() == context.DeadlineExceeded {
			errText = "subagent timed out"
		}
		content = "Subagent failed: " + errText
	case out.Cancelled:
		state = "cancelled"
		isError = true
		content = "Subagent was cancelled."
	default:
		if strings.TrimSpace(content) == "" {
			content = "(the subagent returned no text)"
		}
	}
	if state == "cancelled" {
		content = "Subagent was cancelled."
	}
	duration := time.Since(started).Milliseconds()

	finishedMeta := map[string]any{
		"child_session_id": childID, "state": state,
		"parent_session_id": parentID, "delegation_depth": depth,
		"duration_ms": duration, "turn_count": out.Steps, "usage": out.Usage,
	}
	if errText != "" {
		finishedMeta["error"] = errText
	}
	finalCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, _ = p.appendFacts(finalCtx, parentID, []factIn{{
		Type: "subagent_finished", Role: "host", Meta: mustJSON(finishedMeta),
	}})
	cancel()

	return toolResult{
		Content: content, IsError: isError,
		Meta: map[string]any{
			"child_session_id": childID, "state": state,
			"duration_ms": duration, "turn_count": out.Steps, "usage": out.Usage,
		},
	}, nil
}

func (p *plugin) callTurn(
	ctx context.Context,
	childID string,
	parent sessionInfoOut,
	depth int,
	prompt string,
	allowed []string,
	maxSteps int,
) (turnOut, error) {
	raw, err := p.sdk.Call(ctx, "loop", "turn", mustJSON(turnIn{
		Input: prompt, SessionID: childID,
		WorkspaceID: parent.WorkspaceID, Workspace: parent.Workspace,
		Model: parent.Model, ContextWindow: parent.ContextWindow, MaxSteps: maxSteps,
		AllowedTools: allowed, ParentSessionID: parent.SessionID,
		Origin: "subagent", DelegationDepth: depth,
	}))
	if err != nil {
		return turnOut{}, err
	}
	var out turnOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return turnOut{}, fmt.Errorf("bad loop.turn response: %w", err)
	}
	return out, nil
}

func (p *plugin) createChildSession(ctx context.Context, parent sessionInfoOut, depth int, prompt string) (string, error) {
	raw, err := p.sdk.Call(ctx, "session", "create", mustJSON(sessionCreateIn{
		Name:        "subagent: " + shortLabel(prompt, 48),
		WorkspaceID: parent.WorkspaceID, Workspace: parent.Workspace,
		Model: parent.Model, ContextWindow: parent.ContextWindow,
		ParentSessionID: parent.SessionID, Origin: "subagent", DelegationDepth: depth,
	}))
	if err != nil {
		return "", err
	}
	var out sessionCreateOut
	if err := json.Unmarshal(raw, &out); err != nil || out.SessionID == "" {
		return "", fmt.Errorf("bad session.create response")
	}
	return out.SessionID, nil
}

func (p *plugin) sessionInfo(ctx context.Context, id string) (sessionInfoOut, error) {
	raw, err := p.sdk.Call(ctx, "session", "info", mustJSON(map[string]string{"session_id": id}))
	if err != nil {
		return sessionInfoOut{}, err
	}
	var out sessionInfoOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return sessionInfoOut{}, err
	}
	return out, nil
}

func (p *plugin) appendFacts(ctx context.Context, sessionID string, facts []factIn) (appendOut, error) {
	raw, err := p.sdk.Call(ctx, "session", "append", mustJSON(appendIn{SessionID: sessionID, Facts: facts}))
	if err != nil {
		return appendOut{}, err
	}
	var out appendOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return appendOut{}, err
	}
	return out, nil
}

// childTools gives the child the same tool surface as the parent, minus the
// delegation tool so it cannot nest.
func (p *plugin) childTools(ctx context.Context) ([]string, error) {
	raw, err := p.sdk.Call(ctx, "tools", "list", json.RawMessage(`{}`))
	if err != nil {
		return nil, errCoded(codeToolsUnavailable, "tools.list: "+err.Error())
	}
	var listed toolsListOut
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, errCoded(codeToolsUnavailable, "decode tools.list: "+err.Error())
	}
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		if tool.Name == "subagent" || strings.HasPrefix(tool.Name, "subagent_") {
			continue
		}
		names = append(names, tool.Name)
	}
	if len(names) == 0 {
		return []string{noToolsSentinel}, nil
	}
	return names, nil
}

func shortLabel(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
