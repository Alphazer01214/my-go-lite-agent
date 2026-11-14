package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleSessionCreate(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := p.consumeSessionCreate(ctx, req); err != nil {
		return nil, err
	}
	return nil, nil
}

func (p *plugin) handleStream(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := p.consumeLLMStream(ctx, req); err != nil {
		return nil, err
	}
	return nil, nil
}

type compactReq struct {
	SessionID string `json:"session_id"`
}

type compactResp struct {
	SessionID      string `json:"session_id"`
	Compacted      bool   `json:"compacted"`
	Reason         string `json:"reason,omitempty"`
	Method         string `json:"method,omitempty"`
	CoveredFromSeq int    `json:"covered_from_seq,omitempty"`
	CoveredToSeq   int    `json:"covered_to_seq,omitempty"`
	TokensBefore   int    `json:"tokens_before,omitempty"`
	TokensAfter    int    `json:"tokens_after,omitempty"`
	OverBudget     bool   `json:"over_budget,omitempty"`
}

func (p *plugin) handleCompact(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input compactReq
	contextID := ""
	if req != nil {
		contextID = req.ContextID
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &input); err != nil {
				return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
			}
		}
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "session_id is required")
	}

	info, err := p.sessionInfo(ctx, sessionID)
	if err != nil {
		return nil, prompterError("read session info", err)
	}
	facts, err := p.sessionFacts(ctx, sessionID)
	if err != nil {
		return nil, prompterError("read session facts", err)
	}
	model := strings.TrimSpace(info.Model)
	baseline, err := p.baselineFacts(info.Workspace, model)
	if err != nil {
		return nil, prompterError("render prompt", err)
	}
	facts, err = p.writeBaseline(ctx, sessionID, facts, baseline)
	if err != nil {
		return nil, prompterError("write baseline facts", err)
	}
	tools, err := p.modelTools(ctx)
	if err != nil {
		return nil, prompterError("load tool definitions", err)
	}

	messages := projectMessages(facts)
	result, _, compactedMessages, err := p.compactContext(ctx, compactRequest{
		SessionID: sessionID,
		Model:     model,
		ContextID: contextID,
		Facts:     facts,
		Messages:  messages,
		Tools:     tools,
		Manual:    true,
	})
	if err != nil {
		return nil, err
	}
	if result.Compacted {
		snapshot := buildSnapshotMeta(sessionID, model, baseline, compactedMessages, tools)
		p.writeSnapshot(ctx, sessionID, snapshot)
		p.rememberContext(sessionID, "", model, compactedMessages, tools, snapshot)
	}
	return json.Marshal(compactResp{
		SessionID:      sessionID,
		Compacted:      result.Compacted,
		Reason:         result.Reason,
		Method:         result.Method,
		CoveredFromSeq: result.CoveredFromSeq,
		CoveredToSeq:   result.CoveredToSeq,
		TokensBefore:   result.TokensBefore,
		TokensAfter:    result.TokensAfter,
		OverBudget:     result.OverBudget,
	})
}
