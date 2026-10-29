package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// create session.create
func (p *plugin) handleCreate(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input createReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.create(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// info session.info
func (p *plugin) handleInfo(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input infoReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.info(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// append session.append
func (p *plugin) handleAppend(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input appendReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.appendFacts(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// facts session.facts
func (p *plugin) handleFacts(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input factsReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.facts(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// list session.list
func (p *plugin) handleList(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input listReq
	if len(req.Payload) > 0 {
		if err := pluginsdk.Decode(req, &input); err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
	}
	result, err := p.listSessions(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// workspace.list
func (p *plugin) handleWorkspaceList(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	result, err := p.listWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// workspace.create
func (p *plugin) handleWorkspaceCreate(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input workspaceReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.createWorkspace(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// workspace.update
func (p *plugin) handleWorkspaceUpdate(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input workspaceReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.updateWorkspace(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// workspace.select
func (p *plugin) handleWorkspaceSelect(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input workspaceReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.selectWorkspace(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// workspace.delete
func (p *plugin) handleWorkspaceDelete(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input workspaceReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.deleteWorkspace(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// workspace.current
func (p *plugin) handleWorkspaceCurrent(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	result, err := p.currentWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
