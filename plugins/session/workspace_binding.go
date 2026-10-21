package main

import (
	"fmt"
	"os"
	"strings"
)

func (p *plugin) resolveWorkspaceBinding(workspaceID, workspacePath string) (string, string, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspaceID != "" {
		record, ok := p.workspaces.info(workspaceID)
		if !ok {
			return "", "", errCoded("workspace_not_found", "workspace not found: "+workspaceID)
		}
		if workspacePath != "" && !sameWorkspacePath(workspacePath, record.Path) {
			return "", "", errCoded("workspace_mismatch", "workspace path does not match workspace_id")
		}
		return record.WorkspaceID, record.Path, nil
	}
	if workspacePath == "" {
		return "", "", errCoded("workspace_required", "workspace_id or workspace is required")
	}
	record, _, err := p.workspaces.ensure(workspacePath, "", false)
	if err != nil {
		return "", "", errCoded("workspace_invalid", err.Error())
	}
	return record.WorkspaceID, record.Path, nil
}

// backfillWorkspaces binds sessions that were logged before the workspace
// registry existed. It writes, so it runs once at startup — read handlers must
// stay side-effect free.
func (p *plugin) backfillWorkspaces() {
	p.mu.RLock()
	stores := make([]*store, 0, len(p.reg))
	for _, st := range p.reg {
		stores = append(stores, st)
	}
	p.mu.RUnlock()
	for _, st := range stores {
		meta := st.snapshotMeta()
		_, workspaceKnown := p.workspaces.info(meta.WorkspaceID)
		if (meta.WorkspaceID != "" && workspaceKnown) || strings.TrimSpace(meta.Workspace) == "" {
			continue
		}
		workspaceID, workspacePath, err := p.resolveWorkspaceBinding("", meta.Workspace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: bind workspace for %s: %v\n", st.id, err)
			continue
		}
		// A dangling id was replaced by a fresh one; drop the stale selection
		// entry so it cannot keep pointing at a workspace that no longer exists.
		p.mu.Lock()
		if meta.WorkspaceID != "" && meta.WorkspaceID != workspaceID {
			delete(p.selectedByWorkspace, meta.WorkspaceID)
		}
		p.mu.Unlock()
		meta.WorkspaceID = workspaceID
		meta.Workspace = workspacePath
		if err := st.updateMeta(meta); err != nil {
			fmt.Fprintf(os.Stderr, "session: persist workspace for %s: %v\n", st.id, err)
		}
	}
}

func (p *plugin) selectedWorkspace() (*workspaceRecord, bool) {
	return p.workspaces.current()
}

func (p *plugin) workspaceRecords() ([]workspaceRecord, string) {
	return p.workspaces.list()
}

// setWorkspace changes the workspace registry only. Internal callers that
// already know the target session use this to avoid a second selection pass.
func (p *plugin) setWorkspace(workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return errCoded("workspace_required", "workspace_id is required")
	}
	_, err := p.workspaces.selectID(workspaceID)
	return err
}

// selectWorkspace is the user-facing switch. The pane on screen must belong to
// the new workspace, or the history list and chat context disagree.
func (p *plugin) selectWorkspace(workspaceID string) error {
	if err := p.setWorkspace(workspaceID); err != nil {
		return err
	}
	p.reconcileWorkspaceSelection(workspaceID)
	return nil
}

// reconcileWorkspaceSelection keeps the selected pane inside workspaceID. It
// prefers the workspace's remembered session, then the newest session. An empty
// workspace clears the selection instead of leaving a foreign pane selected.
func (p *plugin) reconcileWorkspaceSelection(workspaceID string) {
	if strings.TrimSpace(workspaceID) == "" {
		return
	}
	p.mu.RLock()
	current := p.selected
	remembered := p.selectedByWorkspace[workspaceID]
	p.mu.RUnlock()

	if current != "" {
		if st, err := p.get(current); err == nil && st.snapshotMeta().WorkspaceID == workspaceID {
			return
		}
	}
	if remembered != "" {
		if st, err := p.get(remembered); err == nil && st.snapshotMeta().WorkspaceID == workspaceID {
			p.mu.Lock()
			p.openSessionLocked(remembered)
			p.selected = remembered
			p.mu.Unlock()
			return
		}
	}

	list, err := p.list(workspaceID)
	if err != nil || len(list.Sessions) == 0 {
		p.mu.Lock()
		p.selected = ""
		if p.selectedByWorkspace[workspaceID] == remembered {
			delete(p.selectedByWorkspace, workspaceID)
		}
		p.mu.Unlock()
		return
	}
	id := list.Sessions[0].SessionID
	p.mu.Lock()
	p.openSessionLocked(id)
	p.selected = id
	p.selectedByWorkspace[workspaceID] = id
	p.mu.Unlock()
}

func sameWorkspacePath(left, right string) bool {
	leftPath, leftErr := canonicalWorkspacePath(left)
	rightPath, rightErr := canonicalWorkspacePath(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return workspacePathKey(leftPath) == workspacePathKey(rightPath)
}
