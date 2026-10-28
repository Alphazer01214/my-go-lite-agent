package main

import (
	"context"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func handleJSON[Req any, Resp any](
	fn func(context.Context, *Req) (*Resp, error),
) pluginsdk.HandleFunc {
	return pluginsdk.Handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var input Req
		if err := pluginsdk.Decode(req, &input); err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
		return fn(ctx, &input)
	})
}

// handleJSONOptional 允许无 payload 的读取方法，例如 workspace.list。
func handleJSONOptional[Req any, Resp any](
	fn func(context.Context, *Req) (*Resp, error),
) pluginsdk.HandleFunc {
	return pluginsdk.Handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var input Req
		if len(req.Payload) > 0 {
			if err := pluginsdk.Decode(req, &input); err != nil {
				return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
			}
		}
		return fn(ctx, &input)
	})
}

// create session.create
func (p *plugin) handleCreate(ctx context.Context, req *createReq) (*Meta, error) {
	return p.create(ctx, req)
}

// info session.info
func (p *plugin) handleInfo(ctx context.Context, req *infoReq) (*infoResp, error) {
	return p.info(ctx, req)
}

// append session.append
func (p *plugin) handleAppend(ctx context.Context, req *appendReq) (*appendResp, error) {
	return p.appendFacts(ctx, req)
}

// facts session.facts
func (p *plugin) handleFacts(ctx context.Context, req *factsReq) (*factsResp, error) {
	return p.facts(ctx, req)
}

// list session.list
func (p *plugin) handleList(ctx context.Context, req *listReq) (*listResp, error) {
	return p.listSessions(ctx, req)
}

// workspace.list
func (p *plugin) handleWorkspaceList(ctx context.Context, _ *workspaceReq) (*workspaceListResp, error) {
	return p.listWorkspaces(ctx)
}

// workspace.create
func (p *plugin) handleWorkspaceCreate(ctx context.Context, req *workspaceReq) (*workspaceCreateResp, error) {
	return p.createWorkspace(ctx, req)
}

// workspace.update
func (p *plugin) handleWorkspaceUpdate(ctx context.Context, req *workspaceReq) (*workspaceUpdateResp, error) {
	return p.updateWorkspace(ctx, req)
}

// workspace.select
func (p *plugin) handleWorkspaceSelect(ctx context.Context, req *workspaceReq) (*workspaceSelectResp, error) {
	return p.selectWorkspace(ctx, req)
}

// workspace.delete
func (p *plugin) handleWorkspaceDelete(ctx context.Context, req *workspaceReq) (*workspaceDeleteResp, error) {
	return p.deleteWorkspace(ctx, req)
}

// workspace.current
func (p *plugin) handleWorkspaceCurrent(ctx context.Context, _ *workspaceReq) (*workspaceCurrentResp, error) {
	return p.currentWorkspace(ctx)
}
