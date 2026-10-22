package main

import (
	"context"
)

func dockStateFromPane(pane chatStateUI) dockStateUI {
	return dockStateUI{
		SessionID:     pane.SessionID,
		Name:          pane.Name,
		WorkspaceID:   pane.WorkspaceID,
		Workspace:     pane.Workspace,
		Model:         pane.Model,
		ContextWindow: pane.ContextWindow,
		Turn:          pane.Turn,
		Usage:         pane.Usage,
		Context:       pane.Context,
		Request:       pane.Request,
	}
}

func (p *plugin) dockState(ctx context.Context, in stateIn) (dockStateUI, error) {
	workspaceID := ""
	if current, ok := p.selectedWorkspace(); ok {
		workspaceID = current.WorkspaceID
	}
	if in.SessionID != "" {
		st, err := p.get(in.SessionID)
		if err != nil {
			return dockStateUI{}, err
		}
		meta := st.snapshotMeta()
		workspaceID = meta.WorkspaceID
		if workspaceID != "" {
			_ = p.setWorkspace(workspaceID)
		}
	}
	id := p.effectiveID(in.SessionID, workspaceID)
	if id == "" {
		return dockStateUI{Revision: p.nextUIRevision()}, nil
	}
	pane, err := p.chatState(ctx, id)
	if err != nil {
		return dockStateUI{}, err
	}
	out := dockStateFromPane(pane)
	out.Revision = p.nextUIRevision()
	return out, nil
}
