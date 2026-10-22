package main

import (
	"context"
	"encoding/json"
)

func (p *plugin) derive(ctx context.Context, contextID, sessionID string) (deriveOut, error) {
	raw, err := p.call(ctx, contextID, "session", "derive", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return deriveOut{}, err
	}
	var out deriveOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return deriveOut{}, errCoded(codeContextDeriveFailed, "decode session.derive: "+err.Error())
	}
	if out.Messages == nil {
		out.Messages = []derivedMessage{}
	}
	return out, nil
}

func (p *plugin) appendFacts(ctx context.Context, contextID, sessionID string, facts []factIn) (appendOut, error) {
	raw, err := p.call(ctx, contextID, "session", "append", mustJSON(appendIn{SessionID: sessionID, Facts: facts}))
	if err != nil {
		return appendOut{}, errCoded(codeContextRecordFailed, err.Error())
	}
	var out appendOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return appendOut{}, errCoded(codeContextRecordFailed, "decode session.append: "+err.Error())
	}
	return out, nil
}

func (p *plugin) queryMessages(ctx context.Context, contextID, sessionID string) ([]fact, error) {
	raw, err := p.call(ctx, contextID, "session", "query", mustJSON(queryIn{
		SessionID: sessionID, Types: []string{factTypeMessage},
	}))
	if err != nil {
		return nil, errCoded(codeContextDeriveFailed, err.Error())
	}
	var out queryOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errCoded(codeContextDeriveFailed, "decode session.query: "+err.Error())
	}
	return out.Facts, nil
}

func (p *plugin) sessionOrigin(ctx context.Context, contextID, sessionID string) (string, []string) {
	raw, err := p.call(ctx, contextID, "session", "info", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return "", []string{"session_info_unavailable"}
	}
	var out struct {
		Origin string `json:"origin,omitempty"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", []string{"session_info_unavailable"}
	}
	return out.Origin, nil
}

func (p *plugin) queryTypedFacts(ctx context.Context, contextID, sessionID string, types []string) ([]fact, error) {
	raw, err := p.call(ctx, contextID, "session", "query", mustJSON(queryIn{
		SessionID: sessionID, Types: types,
	}))
	if err != nil {
		return nil, errCoded(codeContextDeriveFailed, err.Error())
	}
	var out queryOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errCoded(codeContextDeriveFailed, "decode session.query: "+err.Error())
	}
	return out.Facts, nil
}
