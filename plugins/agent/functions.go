package main

import (
	"context"
	"encoding/json"
)

func (p *plugin) turn(ctx context.Context, req *fromWebUI) (*sessionInfoResp, error) {
	ireq, err := json.Marshal(sessionInfoReq{
		SessionId: req.SessionId,
	})
	if err != nil {
		return nil, err
	}
	// 1. check if the session exists
	infoByte, err := p.sdk.Call(ctx, "session", "info", ireq)
	if err != nil {
		// 说明没有 session，开始创建
		creq, err := json.Marshal(sessionCreateReq{
			Title:     "New Session",
			Workspace: req.Workspace,
			Origin:    "agent",
			Depth:     0,
		})
		if err != nil {
			return nil, err
		}
		infoByte, err = p.sdk.Call(ctx, "session", "create", creq)
		if err != nil {
			return nil, err
		}
	}
	var info sessionInfoResp
	if err := json.Unmarshal(infoByte, &info); err != nil {
		return nil, err
	}

	// 2. turn start
	p.sdk.CallWithCallback(ctx, "llm", "stream", nil, nil)
	return &info, nil
}
