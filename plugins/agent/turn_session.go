package main

import (
	"context"
	"encoding/json"
)

func (p *plugin) createSession(ctx context.Context, in turnIn) (string, error) {
	raw, err := p.sdk.Call(ctx, "session", "create", mustJSON(sessionCreateIn{
		WorkspaceID: in.WorkspaceID, Workspace: in.Workspace,
		Model: in.Model, ContextWindow: in.ContextWindow,
		ParentSessionID: in.ParentSessionID,
		Origin:          in.Origin, DelegationDepth: in.DelegationDepth,
	}))
	if err != nil {
		return "", err
	}
	var out sessionCreateOut
	if err := json.Unmarshal(raw, &out); err != nil || out.SessionID == "" {
		return "", errHandler("bad session.create response")
	}
	return out.SessionID, nil
}

func (p *plugin) sessionInfo(ctx context.Context, sessionID string) (sessionInfoOut, error) {
	raw, err := p.sdk.Call(ctx, "session", "info", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return sessionInfoOut{}, err
	}
	var out sessionInfoOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return sessionInfoOut{}, errHandler("bad session.info response")
	}
	return out, nil
}

func (p *plugin) appendFacts(ctx context.Context, sessionID string, facts []map[string]any) ([]int, error) {
	raw, err := p.sdk.Call(ctx, "session", "append", mustJSON(sessionAppendIn{SessionID: sessionID, Facts: facts}))
	if err != nil {
		return nil, err
	}
	var out sessionAppendOut
	_ = json.Unmarshal(raw, &out)
	return out.Seqs, nil
}

func (p *plugin) assembleMessages(ctx context.Context, sessionID string) ([]chatMessage, error) {
	raw, err := p.sdk.Call(ctx, "session", "derive", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return nil, err
	}
	var out messagesOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errHandler("bad session.derive response")
	}
	return out.Messages, nil
}

func (p *plugin) buildModelContext(ctx context.Context, in turnIn, scheme string) (modelContextOut, error) {
	messages, err := p.assembleMessages(ctx, in.SessionID)
	if err != nil {
		return modelContextOut{}, err
	}
	specs, tools, degraded, toolErr := p.collectTools(ctx, scheme, in.AllowedTools, in.DeniedTools)
	if toolErr != nil {
		return modelContextOut{}, toolErr
	}
	return modelContextOut{
		Messages: messages, Tools: tools, ToolSpecs: specs, Degraded: degraded,
		TokenEstimate: estimateContextTokens(messages, tools),
	}, nil
}
