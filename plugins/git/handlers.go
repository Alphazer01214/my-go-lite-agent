package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func decodeRequest(req *pluginsdk.Request, dst any) error {
	if req == nil || len(req.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(req.Payload, dst); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	return nil
}

func (p *plugin) handleStatus(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input statusReq
	if err := decodeRequest(req, &input); err != nil {
		return nil, err
	}
	result, err := p.status(ctx, input.Workspace)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleBranches(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input branchesReq
	if err := decodeRequest(req, &input); err != nil {
		return nil, err
	}
	result, err := p.branches(ctx, input.Workspace)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleDiff(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input diffReq
	if err := decodeRequest(req, &input); err != nil {
		return nil, err
	}
	result, err := p.diff(ctx, input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleCheckout(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input checkoutReq
	if err := decodeRequest(req, &input); err != nil {
		return nil, err
	}
	result, err := p.checkout(ctx, input.Workspace, input.Branch)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleBranchCreate(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input branchCreateReq
	if err := decodeRequest(req, &input); err != nil {
		return nil, err
	}
	result, err := p.createBranch(ctx, input.Workspace, input.Name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleBranchDelete(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input branchDeleteReq
	if err := decodeRequest(req, &input); err != nil {
		return nil, err
	}
	result, err := p.deleteBranch(ctx, input.Workspace, input.Name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
