package main

import (
	"context"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// resolveWorkspace 决定新会话的归属：显式路径优先，否则取当前选中项。
func (p *plugin) resolveWorkspace(workspacePath string) (Workspace, error) {
	workspacePath = strings.TrimSpace(workspacePath)

	if workspacePath != "" {
		record, _, err := p.workspaces.ensure(workspacePath, "", false)
		if err != nil {
			return Workspace{}, errCoded(codeWorkspaceInvalid, err.Error())
		}
		return record, nil
	}
	selected, ok := p.workspaces.current()
	if !ok {
		return Workspace{}, errCoded(codeWorkspaceRequired, "workspace is required: pass an absolute path in workspace, or select one via workspace.select")
	}
	return *selected, nil
}

func (p *plugin) listWorkspaces(ctx context.Context) (*workspaceListResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records, selected := p.workspaces.list()
	return &workspaceListResp{Workspaces: records, Selected: selected}, nil
}

func (p *plugin) createWorkspace(ctx context.Context, req *workspaceReq) (*workspaceCreateResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record, created, err := p.workspaces.ensure(req.Path, req.Name, true)
	if err != nil {
		return nil, errCoded(codeWorkspaceInvalid, err.Error())
	}
	p.loadWorkspace(record.Path)
	return &workspaceCreateResp{
		Workspace: record, Created: created, Selected: p.workspaces.selectedPath(),
	}, nil
}

func (p *plugin) updateWorkspace(ctx context.Context, req *workspaceReq) (*workspaceUpdateResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, errCoded(codeWorkspaceRequired, "path is required")
	}
	if _, ok := p.workspaces.info(path); !ok {
		return nil, errWorkspaceNotFound(path)
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "name is required")
	}
	record, err := p.workspaces.rename(path, req.Name)
	if err != nil {
		return nil, errHandler(err)
	}
	return &workspaceUpdateResp{Workspace: record}, nil
}

func (p *plugin) selectWorkspace(ctx context.Context, req *workspaceReq) (*workspaceSelectResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, errCoded(codeWorkspaceRequired, "path is required")
	}
	record, err := p.workspaces.selectPath(path)
	if err != nil {
		return nil, errWorkspaceNotFound(path)
	}
	p.loadWorkspace(record.Path)
	return &workspaceSelectResp{Workspace: record, Selected: record.Path}, nil
}

func (p *plugin) deleteWorkspace(ctx context.Context, req *workspaceReq) (*workspaceDeleteResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, errCoded(codeWorkspaceRequired, "path is required")
	}
	if _, ok := p.workspaces.info(path); !ok {
		return nil, errWorkspaceNotFound(path)
	}
	if p.hasSessionsInWorkspace(path) {
		return nil, errCoded(codeWorkspaceNotEmpty, "workspace still has sessions: "+path)
	}
	removed, selected, err := p.workspaces.remove(path)
	if err != nil {
		return nil, errHandler(err)
	}
	if selected != "" {
		p.loadWorkspace(selected)
	}
	return &workspaceDeleteResp{
		Path: path, Deleted: removed, Selected: selected,
	}, nil
}

func (p *plugin) currentWorkspace(ctx context.Context) (*workspaceCurrentResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record, ok := p.workspaces.current()
	if !ok {
		return &workspaceCurrentResp{}, nil
	}
	return &workspaceCurrentResp{Workspace: record}, nil
}

func (p *plugin) hasSessionsInWorkspace(path string) bool {
	key := workspacePathKey(path)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, meta := range p.index {
		if workspacePathKey(meta.Workspace) == key {
			return true
		}
	}
	return false
}
