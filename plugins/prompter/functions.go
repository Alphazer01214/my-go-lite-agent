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

	sessionID := strings.TrimSpace(in.Meta.SessionID)
	if sessionID == "" {
		return req.Proceed(ctx, nil)
	}

	info, err := p.sessionInfo(ctx, sessionID)
	if err != nil {
		logf("llm.stream passthrough: session.info: %v", err)
		return req.Proceed(ctx, nil)
	}
	facts, err := p.sessionFacts(ctx, sessionID)
	if err != nil {
		logf("llm.stream passthrough: session.facts: %v", err)
		return req.Proceed(ctx, nil)
	}

	model := strings.TrimSpace(info.Model)
	if model == "" {
		model = strings.TrimSpace(in.Model)
	}
	baseline, err := p.baselineFacts(info.Workspace, model)
	if err != nil {
		logf("llm.stream passthrough: render prompt: %v", err)
		return req.Proceed(ctx, nil)
	}
	facts, err = p.writeBaseline(ctx, sessionID, facts, baseline)
	if err != nil {
		logf("llm.stream passthrough: write baseline: %v", err)
		return req.Proceed(ctx, nil)
	}

	messages := projectMessages(facts)
	if len(messages) == 0 {
		return req.Proceed(ctx, nil)
	}
	p.writeSnapshot(ctx, sessionID, model, baseline, messages)

	payload, err := replaceMessages(raw, messages, model)
	if err != nil {
		logf("llm.stream passthrough: encode messages: %v", err)
		return req.Proceed(ctx, nil)
	}
	return req.Proceed(ctx, payload)
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

func replaceMessages(raw map[string]json.RawMessage, messages []pluginsdk.ChatMessage, model string) (json.RawMessage, error) {
	encoded, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	raw["messages"] = encoded
	if model != "" {
		if current, ok := raw["model"]; !ok || len(current) == 0 || string(current) == "null" {
			modelJSON, err := json.Marshal(model)
			if err != nil {
				return nil, err
			}
			raw["model"] = modelJSON
		}
	}
	return json.Marshal(raw)
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

func (p *plugin) writeSnapshot(
	ctx context.Context,
	sessionID string,
	model string,
	baseline []factIn,
	messages []pluginsdk.ChatMessage,
) {
	systemHash, runtimeHash := "", ""
	if len(baseline) > 0 {
		systemHash = factHash(baseline[0])
	}
	if len(baseline) > 1 {
		runtimeHash = factHash(baseline[1])
	}
	meta, err := json.Marshal(snapshotMeta{
		Source:             "prompter",
		SessionID:          sessionID,
		Model:              model,
		SurfaceSeqs:        surfaceSeqs(messages),
		SystemPromptHash:   systemHash,
		RuntimeContextHash: runtimeHash,
		MessagesHash:       messageHash(messages),
		TokenEstimate:      estimateTokens(messages),
		MessageCount:       len(messages),
	})
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
		Types:     []string{factTypeMessage, factTypeSystemPrompt, factTypeRuntimeContext},
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
