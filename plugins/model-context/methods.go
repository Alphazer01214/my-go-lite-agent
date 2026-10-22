package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"time"
)

func (p *plugin) complete(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if len(req.Payload) == 0 {
		return nil, errBadArguments("payload is required")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return nil, errBadArguments("llm.complete payload must be an object")
	}
	rawMeta, ok := payload["meta"]
	if !ok || len(rawMeta) == 0 {
		return nil, errBadArguments("meta is required")
	}
	var meta completeMeta
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		return nil, errBadArguments("invalid meta: " + err.Error())
	}
	if meta.SessionID == "" {
		return nil, errBadArguments("meta.session_id is required")
	}

	var tools []toolDefinition
	if rawTools, ok := payload["tools"]; ok && len(rawTools) > 0 {
		if err := json.Unmarshal(rawTools, &tools); err != nil {
			return nil, errBadArguments("tools must be an array: " + err.Error())
		}
	}

	out, err := p.prepare(ctx, req.ContextID, meta, tools)
	if err != nil {
		return nil, err
	}
	payload["messages"] = mustJSON(out.Messages)
	if _, ok := payload["tools"]; ok {
		payload["tools"] = mustJSON(out.Tools)
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, errHandler("encode llm.complete payload: " + err.Error())
	}
	return req.Proceed(ctx, rewritten)
}

func (p *plugin) inspect(ctx context.Context, contextID string, in inspectIn) (inspectOut, error) {
	if in.ModelContextID == "" && in.SessionID == "" {
		return inspectOut{}, errBadArguments("model_context_id or session_id is required")
	}
	var snap snapshot
	if in.ModelContextID != "" {
		p.mu.Lock()
		found, ok := p.snapshots[in.ModelContextID]
		p.mu.Unlock()
		if !ok {
			return inspectOut{}, errCoded(codeModelContextNotFound, "snapshot is not in the current process")
		}
		snap = found
	} else {
		facts, err := p.queryTypedFacts(ctx, contextID, in.SessionID, []string{
			factTypeMessage, factTypeContextSummary, factTypeContextSnapshot,
		})
		if err != nil {
			return inspectOut{}, err
		}
		found, ok := latestSnapshotFromFacts(facts)
		if !ok {
			return inspectOut{Found: false, Messages: []inspectedMessage{}}, nil
		}
		snap = found
	}

	out := inspectOut{
		Found:    true,
		Snapshot: snap,
		Messages: []inspectedMessage{},
	}
	if snap.SessionID == "" {
		out.Warnings = append(out.Warnings, "snapshot has no session_id; messages cannot be reconstructed")
		return out, nil
	}
	facts, err := p.queryTypedFacts(ctx, contextID, snap.SessionID, []string{
		factTypeMessage, factTypeContextSummary, factTypeContextSnapshot,
	})
	if err != nil {
		// The compact snapshot is still useful even when the log is unreadable.
		out.Warnings = append(out.Warnings, "reconstruction_unavailable")
		return out, nil
	}
	out.Messages, out.Breakdown, out.Warnings, out.Integrity = reconstructSnapshot(snap, facts)
	// Older snapshots predate truncated_seqs. The current derive can supply the
	// missing seqs; only accept the retry when the recorded hash proves it.
	if out.Integrity == "mismatch" && len(snap.TruncatedSeqs) == 0 {
		derived, err := p.derive(ctx, contextID, snap.SessionID)
		if err == nil && len(derived.TruncatedSeqs) > 0 {
			retry := snap
			retry.TruncatedSeqs = derived.TruncatedSeqs
			messages, breakdown, _, integrity := reconstructSnapshot(retry, facts)
			if integrity == "ok" {
				out.Messages = messages
				out.Breakdown = breakdown
				out.Integrity = "ok"
				kept := out.Warnings[:0]
				for _, warning := range out.Warnings {
					if warning != "messages_hash_mismatch" {
						kept = append(kept, warning)
					}
				}
				out.Warnings = append(kept, "truncation_inferred_from_current_derive")
			}
		}
	}
	return out, nil
}

func (p *plugin) listTools(ctx context.Context, contextID string) ([]toolDefinition, []string) {
	raw, err := p.call(ctx, contextID, "tools", "list", json.RawMessage(`{}`))
	if err != nil {
		return []toolDefinition{}, []string{"tools_unavailable"}
	}
	var listed struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return []toolDefinition{}, []string{"tools_unavailable"}
	}
	tools := make([]toolDefinition, 0, len(listed.Tools))
	for _, item := range listed.Tools {
		parameters := item.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		description := item.Description
		description += fmt.Sprintf("\nRisk: %d.", item.Risk)
		tools = append(tools, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name: item.Name, Description: description, Parameters: parameters,
			},
		})
	}
	return tools, nil
}

func (p *plugin) build(ctx context.Context, contextID string, in buildIn) (buildOut, error) {
	if in.SessionID == "" {
		return buildOut{}, errBadArguments("session_id is required")
	}
	view, err := p.derive(ctx, contextID, in.SessionID)
	if err != nil {
		return buildOut{}, err
	}
	tools := in.Tools
	warnings := []string{}
	if tools == nil {
		var toolWarnings []string
		tools, toolWarnings = p.listTools(ctx, contextID)
		warnings = append(warnings, toolWarnings...)
	}
	if tools == nil {
		tools = []toolDefinition{}
	}
	meta := completeMeta{
		SessionID: in.SessionID, TurnID: in.TurnID, StepID: in.StepID,
		Mode: in.Mode, Scheme: in.Scheme, Workspace: in.Workspace, WorkspaceID: in.WorkspaceID,
		Model: in.Model, ContextWindow: in.ContextWindow,
	}
	origin, originWarnings := p.sessionOrigin(ctx, contextID, in.SessionID)
	warnings = append(warnings, originWarnings...)
	asm, err := p.assemble(meta, view, origin)
	if err != nil {
		return buildOut{}, err
	}
	warnings = append(warnings, asm.Warnings...)
	messages := overlayView(view, asm)
	limit := p.contextLimit(in.ContextWindow)
	if in.MaxContextTokens > 0 {
		limit = in.MaxContextTokens
		if in.ContextWindow > 0 && in.ContextWindow < limit {
			limit = in.ContextWindow
		}
	}
	compactNeeded := limit > 0 && estimateTokens(messages, tools) >= int(float64(limit)*p.cfg.CompactThreshold)
	return buildOut{
		ModelContextID:     fmt.Sprintf("mc-preview-%d-%d", time.Now().UnixNano(), p.seq.Add(1)),
		Messages:           messages,
		Tools:              tools,
		TokenEstimate:      estimateTokens(messages, tools),
		CompactNeeded:      compactNeeded,
		Warnings:           dedupeStrings(warnings),
		SystemPromptHash:   asm.SystemHash,
		RuntimeContextHash: asm.RuntimeHash,
	}, nil
}

func (p *plugin) compact(ctx context.Context, contextID string, in compactIn) (compactOut, error) {
	if in.SessionID == "" {
		return compactOut{}, errBadArguments("session_id is required")
	}
	view, err := p.derive(ctx, contextID, in.SessionID)
	if err != nil {
		return compactOut{}, err
	}
	retain := p.cfg.RetainRecentTokens
	if in.KeepRecentTokens > 0 {
		retain = in.KeepRecentTokens
	}
	maxChars := p.cfg.CompactMaxChars
	if in.MaxSummaryChars > 0 {
		maxChars = in.MaxSummaryChars
	}
	tools := p.latestSnapshotTools(in.SessionID)
	return p.compactOnce(ctx, contextID, in.SessionID, retain, maxChars, "manual", view, tools)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || containsString(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}
