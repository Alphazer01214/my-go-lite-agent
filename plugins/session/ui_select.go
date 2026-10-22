package main

import (
	"context"
	"strings"
)

func (p *plugin) selectedID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.selected
}

func (p *plugin) setSelected(id string) {
	p.mu.Lock()
	p.selected = id
	p.mu.Unlock()
}

func (p *plugin) isOpenLocked(id string) bool {
	for _, value := range p.open {
		if value == id {
			return true
		}
	}
	return false
}

func (p *plugin) isOpen(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isOpenLocked(id)
}

func (p *plugin) openSessionLocked(id string) bool {
	if p.isOpenLocked(id) {
		return false
	}
	p.open = append(p.open, id)
	return true
}

func (p *plugin) openIDsLocked(workspaceID string) []string {
	out := make([]string, 0, len(p.open))
	for _, id := range p.open {
		st := p.reg[id]
		if st == nil {
			continue
		}
		if workspaceID == "" || st.snapshotMeta().WorkspaceID == workspaceID {
			out = append(out, id)
		}
	}
	return out
}

func (p *plugin) openIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.open...)
}

func (p *plugin) openIDsFor(workspaceID string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.openIDsLocked(workspaceID)
}

func (p *plugin) selectSession(ctx context.Context, in selectIn) (selectOut, error) {
	if in.SessionID == "" {
		return selectOut{}, errBadArguments("session_id is required")
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return selectOut{}, err
	}
	meta := st.snapshotMeta()
	if err := p.setWorkspace(meta.WorkspaceID); err != nil {
		return selectOut{}, err
	}
	p.mu.Lock()
	p.openSessionLocked(in.SessionID)
	p.selected = in.SessionID
	if meta.WorkspaceID != "" {
		p.selectedByWorkspace[meta.WorkspaceID] = in.SessionID
	}
	p.mu.Unlock()
	p.publishForSelection(ctx)
	return selectOut{SessionID: in.SessionID}, nil
}

func (p *plugin) closeSession(ctx context.Context, in closeIn) (closeOut, error) {
	if in.SessionID == "" {
		return closeOut{}, errBadArguments("session_id is required")
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return closeOut{}, err
	}
	meta := st.snapshotMeta()
	p.mu.Lock()
	index := -1
	for i, id := range p.open {
		if id == in.SessionID {
			index = i
			break
		}
	}
	if index < 0 {
		selected := p.selected
		p.mu.Unlock()
		return closeOut{SessionID: in.SessionID, Closed: false, Selected: selected}, nil
	}
	neighbors := p.openIDsLocked(meta.WorkspaceID)
	position := 0
	for i, id := range neighbors {
		if id == in.SessionID {
			position = i
			break
		}
	}
	p.open = append(p.open[:index], p.open[index+1:]...)
	wasWorkspaceSelection := meta.WorkspaceID != "" && p.selectedByWorkspace[meta.WorkspaceID] == in.SessionID
	switch {
	case p.selected == in.SessionID:
		remaining := p.openIDsLocked(meta.WorkspaceID)
		switch {
		case len(remaining) == 0:
			p.selected = ""
		case position < len(remaining):
			p.selected = remaining[position]
		default:
			p.selected = remaining[len(remaining)-1]
		}
		if p.selected != "" && meta.WorkspaceID != "" {
			p.selectedByWorkspace[meta.WorkspaceID] = p.selected
		} else if wasWorkspaceSelection {
			delete(p.selectedByWorkspace, meta.WorkspaceID)
		}
	case wasWorkspaceSelection:
		// Not the active session, but it was this workspace's selection. The
		// entry must move to a still-open session of the same workspace or be
		// dropped: leaving the closed id would make effectiveID() resolve to a
		// session that no longer exists.
		remaining := p.openIDsLocked(meta.WorkspaceID)
		if len(remaining) > 0 {
			p.selectedByWorkspace[meta.WorkspaceID] = remaining[0]
		} else {
			delete(p.selectedByWorkspace, meta.WorkspaceID)
		}
	}
	selected := p.selected
	p.mu.Unlock()
	p.publishForSelection(ctx)
	return closeOut{SessionID: in.SessionID, Closed: true, Selected: selected}, nil
}

func (p *plugin) updateSession(ctx context.Context, in updateIn) (updateOut, error) {
	if in.SessionID == "" {
		return updateOut{}, errBadArguments("session_id is required")
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return updateOut{}, err
	}
	meta := st.snapshotMeta()
	changed := false
	if in.Workspace != nil || in.WorkspaceID != nil {
		return updateOut{}, errBadArguments("workspace is immutable for an existing session")
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if meta.Name != name {
			meta.Name = name
			changed = true
		}
	}
	if in.Pinned != nil && meta.Pinned != *in.Pinned {
		meta.Pinned = *in.Pinned
		changed = true
	}
	if changed {
		if err := st.updateMeta(meta); err != nil {
			return updateOut{}, err
		}
		p.publishHistory(ctx)
		if p.isOpen(in.SessionID) {
			p.publishPanes(ctx)
		}
		if p.selectedID() == in.SessionID {
			p.publishTrace(ctx, in.SessionID)
		}
	}
	return updateOut{
		SessionID: in.SessionID, Name: meta.Name, Pinned: meta.Pinned, Workspace: meta.Workspace,
	}, nil
}

func (p *plugin) effectiveID(requested, workspaceID string) string {
	if requested != "" {
		return requested
	}
	if id := p.selectedID(); id != "" && p.isOpen(id) {
		return id
	}
	p.mu.RLock()
	id := p.selectedByWorkspace[workspaceID]
	p.mu.RUnlock()
	if id != "" && p.isOpen(id) {
		return id
	}
	list, err := p.list(workspaceID, false)
	if err != nil || len(list.Sessions) == 0 {
		return ""
	}
	id = list.Sessions[0].SessionID
	p.mu.Lock()
	p.openSessionLocked(id)
	p.selected = id
	p.mu.Unlock()
	return id
}
