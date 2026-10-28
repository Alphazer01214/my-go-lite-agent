package main

import (
	"context"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// resolveWorkspace 决定新会话的归属：显式 id 优先，其次显式路径，最后取当前选中项。
func (p *plugin) resolveWorkspace(workspaceId, workspacePath string) (Workspace, error) {
	workspaceId = strings.TrimSpace(workspaceId)
	workspacePath = strings.TrimSpace(workspacePath)

	if workspaceId != "" {
		record, ok := p.workspaces.info(workspaceId)
		if !ok {
			return Workspace{}, errCoded(codeWorkspaceNotFound, "workspace not found: "+workspaceId)
		}
		if workspacePath != "" && !sameWorkspacePath(workspacePath, record.Path) {
			return Workspace{}, errCoded(codeWorkspaceMismatch, "workspace path does not match workspace_id")
		}
		return record, nil
	}
	if workspacePath != "" {
		record, _, err := p.workspaces.ensure(workspacePath, "", false)
		if err != nil {
			return Workspace{}, errCoded(codeWorkspaceInvalid, err.Error())
		}
		return record, nil
	}
	selected, ok := p.workspaces.current()
	if !ok {
		return Workspace{}, errCoded(codeWorkspaceRequired, "workspace_id or workspace is required")
	}
	return *selected, nil
}

func (p *plugin) listWorkspaces(ctx context.Context) (*workspaceListResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records, selected := p.workspaces.list()
	return &workspaceListResp{Workspaces: records, SelectedId: selected}, nil
}

func (p *plugin) createWorkspace(ctx context.Context, req *workspaceReq) (*workspaceCreateResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record, created, err := p.workspaces.ensure(req.Path, req.Name, true)
	if err != nil {
		return nil, errCoded(codeWorkspaceInvalid, err.Error())
	}
	p.loadWorkspace(record.WorkspaceId)
	return &workspaceCreateResp{
		Workspace: record, Created: created, SelectedId: p.workspaces.selectedID(),
	}, nil
}

func (p *plugin) updateWorkspace(ctx context.Context, req *workspaceReq) (*workspaceUpdateResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceId := strings.TrimSpace(req.WorkspaceId)
	if workspaceId == "" {
		return nil, errCoded(codeWorkspaceRequired, "workspace_id is required")
	}
	if _, ok := p.workspaces.info(workspaceId); !ok {
		return nil, errCoded(codeWorkspaceNotFound, "workspace not found: "+workspaceId)
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "name is required")
	}
	if strings.TrimSpace(req.Path) != "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "workspace path is immutable")
	}
	record, err := p.workspaces.rename(workspaceId, req.Name)
	if err != nil {
		return nil, errHandler(err)
	}
	return &workspaceUpdateResp{Workspace: record}, nil
}

func (p *plugin) selectWorkspace(ctx context.Context, req *workspaceReq) (*workspaceSelectResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceId := strings.TrimSpace(req.WorkspaceId)
	if workspaceId == "" {
		return nil, errCoded(codeWorkspaceRequired, "workspace_id is required")
	}
	record, err := p.workspaces.selectID(workspaceId)
	if err != nil {
		return nil, errCoded(codeWorkspaceNotFound, "workspace not found: "+workspaceId)
	}
	p.loadWorkspace(workspaceId)
	return &workspaceSelectResp{Workspace: record, SelectedId: workspaceId}, nil
}

func (p *plugin) deleteWorkspace(ctx context.Context, req *workspaceReq) (*workspaceDeleteResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceId := strings.TrimSpace(req.WorkspaceId)
	if workspaceId == "" {
		return nil, errCoded(codeWorkspaceRequired, "workspace_id is required")
	}
	if _, ok := p.workspaces.info(workspaceId); !ok {
		return nil, errCoded(codeWorkspaceNotFound, "workspace not found: "+workspaceId)
	}
	if p.hasSessionsInWorkspace(workspaceId) {
		return nil, errCoded(codeWorkspaceNotEmpty, "workspace still has sessions: "+workspaceId)
	}
	removed, selected, err := p.workspaces.remove(workspaceId)
	if err != nil {
		return nil, errHandler(err)
	}
	if selected != "" {
		p.loadWorkspace(selected)
	}
	return &workspaceDeleteResp{
		WorkspaceId: workspaceId, Deleted: removed, SelectedId: selected,
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

func (p *plugin) hasSessionsInWorkspace(workspaceId string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, meta := range p.index {
		if meta.WorkspaceId == workspaceId {
			return true
		}
	}
	return false
}

func sameWorkspacePath(left, right string) bool {
	leftPath, leftErr := canonicalWorkspacePath(left)
	rightPath, rightErr := canonicalWorkspacePath(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return workspacePathKey(leftPath) == workspacePathKey(rightPath)
}
