package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"strings"
	"sync"
)

func (p *plugin) resolveModel(ctx context.Context, requested string, storedWindow int) (string, int, string) {
	requested = strings.TrimSpace(requested)
	raw, err := p.sdk.Call(ctx, "llm", "models.list", json.RawMessage("{}"))
	if err != nil {
		return requested, storedWindow, ""
	}
	var catalog modelsListOut
	if err := json.Unmarshal(raw, &catalog); err != nil || len(catalog.Models) == 0 {
		return requested, storedWindow, ""
	}
	if requested != "" {
		for _, model := range catalog.Models {
			if model.ID != requested {
				continue
			}
			if model.ContextWindow > 0 {
				return model.ID, model.ContextWindow, ""
			}
			return model.ID, storedWindow, ""
		}
	}
	def := strings.TrimSpace(catalog.DefaultModel)
	var fallback modelConfig
	found := false
	for _, model := range catalog.Models {
		if model.ID == def {
			fallback = model
			found = true
			break
		}
	}
	if !found {
		fallback = catalog.Models[0]
	}
	if fallback.ID == "" {
		return requested, storedWindow, ""
	}
	if requested == "" {
		return fallback.ID, fallback.ContextWindow, ""
	}
	return fallback.ID, fallback.ContextWindow, requested
}

func estimateContextTokens(messages []chatMessage, tools []toolDefinition) int {
	body, err := json.Marshal(struct {
		Messages []chatMessage    `json:"messages"`
		Tools    []toolDefinition `json:"tools"`
	}{Messages: messages, Tools: tools})
	if err != nil {
		return 0
	}
	return (len(body) + 3) / 4
}

func (p *plugin) collectTools(ctx context.Context, scheme string, allowed, denied []string) ([]toolSpec, []toolDefinition, string, error) {
	degraded := ""
	if len(allowed) == 0 && scheme == schemeChat {
		return nil, nil, "", nil
	}
	raw, err := p.sdk.Call(ctx, "tools", "list", mustJSON(map[string]any{}))
	if err != nil {
		return nil, nil, "tools unavailable: " + err.Error(), nil
	}
	var listed struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, nil, "bad tools.list response: " + err.Error(), nil
	}
	filtered := filterTools(listed.Tools, scheme, allowed)
	if len(denied) > 0 {
		blocked := make(map[string]bool, len(denied))
		for _, name := range denied {
			blocked[name] = true
		}
		kept := filtered[:0]
		for _, tool := range filtered {
			if !blocked[tool.Name] {
				kept = append(kept, tool)
			}
		}
		filtered = kept
	}
	if len(filtered) == 0 {
		return nil, nil, degraded, nil
	}
	tools := make([]toolDefinition, 0, len(filtered))
	for _, t := range filtered {
		params := t.InputSchema
		if len(t.InputSchema) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		description := t.Description
		description += fmt.Sprintf("\nRisk: %d.", t.Risk)
		tools = append(tools, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name:        t.Name,
				Description: description,
				Parameters:  params,
			},
		})
	}
	return filtered, tools, degraded, nil
}

func (p *plugin) callTool(ctx context.Context, name, arguments, sessionID, workspace, workspaceID, mode, callID string) (toolResult, error) {
	args := json.RawMessage(arguments)
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	raw, err := p.sdk.Call(ctx, "tools", "call", mustJSON(map[string]any{
		"name":         name,
		"arguments":    args,
		"session_id":   sessionID,
		"workspace":    workspace,
		"workspace_id": workspaceID,
		"mode":         mode,
		"call_id":      callID,
	}))
	if err != nil {
		return toolResult{}, err
	}
	var out toolResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return toolResult{}, errHandler("bad tools.call result: " + err.Error())
	}
	return out, nil
}

type toolCallOutcome struct {
	content string
	meta    map[string]any
}

func (p *plugin) canRunToolsParallel(specs []toolSpec, calls []toolCall) bool {
	cfg := p.snapshot()
	if cfg.ToolParallelism != "auto" || len(calls) < 2 || cfg.MaxParallelTools < 2 {
		return false
	}
	byName := make(map[string]toolSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	for _, call := range calls {
		spec, ok := byName[call.Function.Name]
		if !ok || !spec.ParallelSafe {
			return false
		}
	}
	return true
}

func (p *plugin) runToolCallsParallel(st *turnState, in turnIn, scheme string, calls []toolCall) (bool, []int, error) {
	limit := p.snapshot().MaxParallelTools
	if limit < 2 {
		limit = 2
	}
	outcomes := make([]toolCallOutcome, len(calls))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		go func(i int, tc toolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, callErr := p.callTool(st.ctx, tc.Function.Name, tc.Function.Arguments,
				in.SessionID, in.Workspace, in.WorkspaceID, scheme, tc.ID)
			content := result.Content
			toolMeta := map[string]any{
				"tool_name": tc.Function.Name, "is_error": result.IsError,
				"truncated": result.Truncated,
			}
			if result.Meta != nil {
				toolMeta["tool_meta"] = result.Meta
			}
			if callErr != nil {
				if code := pluginsdk.Code(callErr); code != "" {
					content = fmt.Sprintf("tool error [%s]: %s", code, callErr.Error())
					toolMeta["error_code"] = code
				} else {
					content = "tool error: " + callErr.Error()
				}
				toolMeta["is_error"] = true
				toolMeta["wire_error"] = callErr.Error()
			}
			outcomes[i] = toolCallOutcome{content: content, meta: toolMeta}
		}(i, tc)
	}
	wg.Wait()

	seqs := make([]int, 0, len(calls))
	cancelled := false
	for i, tc := range calls {
		appendCtx := st.ctx
		var release context.CancelFunc
		if st.isCancelled() || st.ctx.Err() != nil {
			cancelled = true
			appendCtx, release = finalizeContext(st.ctx)
		}
		meta := map[string]any{"turn_id": st.turnID, "plugin": "agent"}
		if st.stepID != "" {
			meta["step_id"] = st.stepID
		}
		for key, value := range outcomes[i].meta {
			meta[key] = value
		}
		appended, err := p.appendFacts(appendCtx, in.SessionID, []map[string]any{{
			"type": "message", "role": "tool", "content": outcomes[i].content,
			"tool_call_id": tc.ID, "meta": meta,
		}})
		if release != nil {
			release()
		}
		if err != nil {
			return cancelled, seqs, err
		}
		seqs = append(seqs, appended...)
	}
	if st.isCancelled() || st.ctx.Err() != nil {
		cancelled = true
	}
	return cancelled, seqs, nil
}
