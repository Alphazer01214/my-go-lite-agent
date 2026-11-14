package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) consumeSessionCreate(ctx context.Context, req *pluginsdk.Request) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(req.Payload, &raw); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	var in createPayload
	_ = json.Unmarshal(req.Payload, &in)

	workspace := strings.TrimSpace(in.Workspace)
	if workspace == "" {
		return req.Proceed(ctx, nil)
	}

	baseline, err := p.baselineFacts(workspace, strings.TrimSpace(in.Model))
	if err != nil {
		logf("session.create baseline skipped: %v", err)
		return req.Proceed(ctx, nil)
	}
	payload, err := mergeCreateFacts(raw, baseline)
	if err != nil {
		logf("session.create facts skipped: %v", err)
		return req.Proceed(ctx, nil)
	}
	return req.Proceed(ctx, payload)
}

func (p *plugin) consumeLLMStream(ctx context.Context, req *pluginsdk.Request) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(req.Payload, &raw); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	var in streamPayload
	_ = json.Unmarshal(req.Payload, &in)

	// The compaction auxiliary call re-enters llm.stream; it must reach the
	// provider untouched instead of being assembled (and compacted) again.
	if in.Purpose == compactPurpose {
		return req.Proceed(ctx, nil)
	}

	sessionID := strings.TrimSpace(in.Meta.SessionID)
	if sessionID == "" {
		return req.Proceed(ctx, nil)
	}

	info, err := p.sessionInfo(ctx, sessionID)
	if err != nil {
		return prompterError("read session info", err)
	}
	facts, err := p.sessionFacts(ctx, sessionID)
	if err != nil {
		return prompterError("read session facts", err)
	}

	model := strings.TrimSpace(info.Model)
	if model == "" {
		model = strings.TrimSpace(in.Model)
	}
	baseline, err := p.baselineFacts(info.Workspace, model)
	if err != nil {
		return prompterError("render prompt", err)
	}
	facts, err = p.writeBaseline(ctx, sessionID, facts, baseline)
	if err != nil {
		return prompterError("write baseline facts", err)
	}
	tools, err := p.modelTools(ctx)
	if err != nil {
		return prompterError("load tool definitions", err)
	}

	messages := projectMessages(facts)
	if len(messages) == 0 {
		return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "prompter has no model-visible messages")
	}
	facts, messages, err = p.maybeCompact(ctx, req, sessionID, model, facts, messages, tools)
	if err != nil {
		return err
	}
	turnID := strings.TrimSpace(in.Meta.TurnID)
	snapshot := buildSnapshotMeta(sessionID, model, baseline, messages, tools)
	p.writeSnapshot(ctx, sessionID, snapshot)
	p.rememberContext(sessionID, turnID, model, messages, tools, snapshot)

	payload, err := replaceModelPayload(raw, messages, model, tools.Tools)
	if err != nil {
		return prompterError("assemble model payload", err)
	}
	return req.Proceed(ctx, payload)
}

func prompterError(scope string, err error) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "prompter "+scope+": "+err.Error())
}

func (p *plugin) maybeCompact(
	ctx context.Context,
	req *pluginsdk.Request,
	sessionID, model string,
	facts []sessionFact,
	messages []pluginsdk.ChatMessage,
	tools toolCatalog,
) ([]sessionFact, []pluginsdk.ChatMessage, error) {
	result, compactedFacts, compactedMessages, err := p.compactContext(ctx, compactRequest{
		SessionID: sessionID,
		Model:     model,
		ContextID: req.ContextID,
		Facts:     facts,
		Messages:  messages,
		Tools:     tools,
	})
	if err != nil {
		return nil, nil, err
	}
	if !result.Compacted {
		return facts, messages, nil
	}
	logf("compacted session %s: %d -> %d tokens (%s)", sessionID, result.TokensBefore, result.TokensAfter, result.Method)
	return compactedFacts, compactedMessages, nil
}

func mergeCreateFacts(raw map[string]json.RawMessage, baseline []factIn) (json.RawMessage, error) {
	var existing []json.RawMessage
	if encoded, ok := raw["facts"]; ok && len(encoded) > 0 && string(encoded) != "null" {
		if err := json.Unmarshal(encoded, &existing); err != nil {
			return nil, err
		}
	}
	merged := make([]json.RawMessage, 0, len(baseline)+len(existing))
	for _, fact := range baseline {
		encoded, err := json.Marshal(fact)
		if err != nil {
			return nil, err
		}
		merged = append(merged, encoded)
	}
	merged = append(merged, existing...)
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	raw["facts"] = encoded
	return json.Marshal(raw)
}

func replaceModelPayload(
	raw map[string]json.RawMessage,
	messages []pluginsdk.ChatMessage,
	model string,
	tools []pluginsdk.Tool,
) (json.RawMessage, error) {
	encodedMessages, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	if tools == nil {
		tools = []pluginsdk.Tool{}
	}
	encodedTools, err := json.Marshal(tools)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{
		"messages": encodedMessages,
		"tools":    encodedTools,
	}
	if model != "" {
		encodedModel, err := json.Marshal(model)
		if err != nil {
			return nil, err
		}
		out["model"] = encodedModel
	} else if current, ok := raw["model"]; ok {
		out["model"] = current
	}
	if meta, ok := raw["meta"]; ok {
		out["meta"] = meta
	}
	return json.Marshal(out)
}

func newToolCatalog(generation int64, tools []pluginsdk.Tool) toolCatalog {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return toolCatalog{
		Generation: generation,
		Tools:      tools,
		Names:      names,
		Hash:       toolCatalogHash(tools),
	}
}

func toolCatalogHash(tools []pluginsdk.Tool) string {
	encoded, err := json.Marshal(tools)
	if err != nil {
		return ""
	}
	return hashText(string(encoded))
}

func (p *plugin) modelTools(ctx context.Context) (toolCatalog, error) {
	raw, err := p.sdk.Call(ctx, "tool", "list", json.RawMessage(`{}`))
	if err != nil {
		return toolCatalog{}, err
	}
	var out toolListResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return toolCatalog{}, err
	}
	tools := make([]pluginsdk.Tool, 0, len(out.Tools))
	for _, tool := range out.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return toolCatalog{}, fmt.Errorf("tool definition has empty name")
		}
		tools = append(tools, pluginsdk.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	return newToolCatalog(out.CatalogGeneration, tools), nil
}

func (p *plugin) writeBaseline(
	ctx context.Context,
	sessionID string,
	facts []sessionFact,
	baseline []factIn,
) ([]sessionFact, error) {
	changed := changedBaselineFacts(facts, baseline)
	if len(changed) == 0 {
		return facts, nil
	}
	if _, err := p.appendFacts(ctx, sessionID, changed); err != nil {
		return nil, err
	}
	return p.sessionFacts(ctx, sessionID)
}

func (p *plugin) writeSnapshot(ctx context.Context, sessionID string, snapshot snapshotMeta) {
	meta, err := json.Marshal(snapshot)
	if err != nil {
		logf("context snapshot skipped: %v", err)
		return
	}
	if _, err := p.appendFacts(ctx, sessionID, []factIn{{
		Type: factTypeContextSnapshot,
		Role: "host",
		Meta: meta,
	}}); err != nil {
		logf("context snapshot skipped: %v", err)
	}
}

func buildSnapshotMeta(
	sessionID string,
	model string,
	baseline []factIn,
	messages []pluginsdk.ChatMessage,
	tools toolCatalog,
) snapshotMeta {
	systemHash, runtimeHash := "", ""
	if len(baseline) > 0 {
		systemHash = factHash(baseline[0])
	}
	if len(baseline) > 1 {
		runtimeHash = factHash(baseline[1])
	}
	return snapshotMeta{
		Source:                "prompter",
		SessionID:             sessionID,
		Model:                 model,
		SurfaceSeqs:           surfaceSeqs(messages),
		SystemPromptHash:      systemHash,
		RuntimeContextHash:    runtimeHash,
		MessagesHash:          messageHash(messages),
		MessageCount:          len(messages),
		ToolCatalogGeneration: tools.Generation,
		ToolCatalogHash:       tools.Hash,
		ToolNames:             tools.Names,
	}
}

func factHash(fact factIn) string {
	if len(fact.Meta) == 0 {
		return hashText(fact.TextContent)
	}
	var meta factMeta
	if err := json.Unmarshal(fact.Meta, &meta); err != nil || meta.Hash == "" {
		return hashText(fact.TextContent)
	}
	return meta.Hash
}

func (p *plugin) sessionInfo(ctx context.Context, sessionID string) (sessionInfo, error) {
	payload, err := json.Marshal(map[string]string{"session_id": sessionID})
	if err != nil {
		return sessionInfo{}, err
	}
	raw, err := p.sdk.Call(ctx, "session", "info", payload)
	if err != nil {
		return sessionInfo{}, err
	}
	var info sessionInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return sessionInfo{}, err
	}
	return info, nil
}

func (p *plugin) sessionFacts(ctx context.Context, sessionID string) ([]sessionFact, error) {
	payload, err := json.Marshal(factsReq{
		SessionID: sessionID,
		Types: []string{
			factTypeMessage,
			factTypeSystemPrompt,
			factTypeRuntimeContext,
			factTypeContextCompaction,
			factTypeUsage,
		},
	})
	if err != nil {
		return nil, err
	}
	raw, err := p.sdk.Call(ctx, "session", "facts", payload)
	if err != nil {
		return nil, err
	}
	var out factsResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Facts, nil
}

func (p *plugin) appendFacts(ctx context.Context, sessionID string, facts []factIn) ([]int, error) {
	payload, err := json.Marshal(appendReq{SessionID: sessionID, Facts: facts})
	if err != nil {
		return nil, err
	}
	raw, err := p.sdk.Call(ctx, "session", "append", payload)
	if err != nil {
		return nil, err
	}
	var out appendResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Seqs, nil
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "prompter: "+format+"\n", args...)
}
