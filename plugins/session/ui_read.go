package main

import (
	"context"
	"encoding/json"
)

func (p *plugin) state(ctx context.Context, in stateIn) (stateOut, error) {
	workspace, hasWorkspace := p.selectedWorkspace()
	workspaceID := ""
	workspaces, selectedWorkspaceID := p.workspaceRecords()
	if hasWorkspace {
		workspaceID = workspace.WorkspaceID
	}
	if in.SessionID != "" {
		if st, err := p.get(in.SessionID); err == nil {
			meta := st.snapshotMeta()
			workspaceID = meta.WorkspaceID
			if workspaceID != "" {
				_ = p.setWorkspace(workspaceID)
			}
		}
	}
	id := p.effectiveID(in.SessionID, workspaceID)
	if id != "" {
		st, err := p.get(id)
		if err != nil {
			// effectiveID may return an id that was never opened or has since
			// been removed; report it instead of dereferencing a nil store.
			return stateOut{}, err
		}
		meta := st.snapshotMeta()
		p.mu.Lock()
		p.openSessionLocked(id)
		p.selected = id
		if meta.WorkspaceID != "" {
			p.selectedByWorkspace[meta.WorkspaceID] = id
		}
		p.mu.Unlock()
	}
	out := stateOut{History: historyStateUI{
		Sessions: []sessionSummary{}, SelectedID: id,
		WorkspaceID: workspaceID, SelectedWorkspaceID: selectedWorkspaceID,
		Workspaces: workspaces,
	}}
	list, err := p.list(workspaceID, false)
	if err != nil {
		return stateOut{}, err
	}
	out.History.Sessions = list.Sessions
	out.History.OpenIDs = p.openIDsFor(workspaceID)
	out.History.Revision = p.nextUIRevision()
	if id == "" {
		return out, nil
	}
	chats := p.uiPaneStates(ctx, workspaceID, id)
	out.Chats = chats
	for i := range chats {
		if chats[i].SessionID == id {
			chat := chats[i]
			out.Chat = &chat
			break
		}
	}
	if out.Chat == nil {
		chat, err := p.chatState(ctx, id)
		if err != nil {
			return stateOut{}, err
		}
		out.Chat = &chat
	}
	facts, _, err := p.readFacts(id)
	if err != nil {
		return stateOut{}, err
	}
	trace := windowedTraceState(id, facts, p.nextUIRevision())
	out.Trace = &trace
	return out, nil
}

func appendedFacts(ins []factIn, seqs []int, timestamp int64) []Fact {
	facts := make([]Fact, 0, len(ins))
	for i, in := range ins {
		seq := 0
		if i < len(seqs) {
			seq = seqs[i]
		}
		facts = append(facts, Fact{
			Seq: seq, Type: in.Type, Role: in.Role, Content: in.Content,
			ToolCalls: in.ToolCalls, ToolCallID: in.ToolCallID, Meta: in.Meta,
			Timestamp: timestamp,
		})
	}
	return facts
}

func (p *plugin) messages(in messagesIn) (messagesOut, error) {
	if in.SessionID == "" {
		return messagesOut{}, errBadArguments("session_id is required")
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return messagesOut{}, err
	}
	facts, _, err := p.readFacts(st.id)
	if err != nil {
		return messagesOut{}, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = chatWindowMessages
	}
	if limit > uiPageLimit {
		limit = uiPageLimit
	}
	all := make([]chatMessageUI, 0, len(facts))
	for _, fact := range facts {
		if fact.Type == TypeMessage {
			all = append(all, chatMessageFromFact(fact))
		}
	}
	end := len(all)
	if in.BeforeSeq > 0 {
		for end > 0 && all[end-1].Seq >= in.BeforeSeq {
			end--
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := make([]chatMessageUI, end-start)
	copy(page, all[start:end])
	out := messagesOut{
		SessionID: st.id, Messages: page, HasMore: start > 0, Total: len(all),
	}
	if len(page) > 0 {
		out.FirstSeq = page[0].Seq
	}
	return out, nil
}

func (p *plugin) facts(in factsIn) (factsOut, error) {
	if in.SessionID == "" {
		return factsOut{}, errBadArguments("session_id is required")
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return factsOut{}, err
	}
	facts, total, err := p.readFacts(st.id)
	if err != nil {
		return factsOut{}, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = traceWindowFacts
	}
	if limit > uiPageLimit {
		limit = uiPageLimit
	}
	end := len(facts)
	if in.BeforeSeq > 0 {
		for end > 0 && facts[end-1].Seq >= in.BeforeSeq {
			end--
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := make([]Fact, end-start)
	copy(page, facts[start:end])
	out := factsOut{
		SessionID: st.id, Facts: page, HasMore: start > 0, Total: total,
	}
	if len(page) > 0 {
		out.FirstSeq = page[0].Seq
	}
	return out, nil
}

func (p *plugin) pluginState(ctx context.Context, name string) string {
	if p.sdk == nil || name == "" {
		return ""
	}
	raw, err := p.sdk.Call(ctx, "host", "plugins", json.RawMessage(`{}`))
	if err != nil {
		return ""
	}
	var out struct {
		Plugins []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"plugins"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return ""
	}
	for _, plugin := range out.Plugins {
		if plugin.Name == name {
			return plugin.State
		}
	}
	return "missing"
}

func (p *plugin) recoverOpenTurn(sessionID, turnID, owner string) bool {
	if turnID == "" {
		return false
	}
	st, err := p.get(sessionID)
	if err != nil {
		return false
	}
	meta := map[string]any{
		"turn_id": turnID,
		"reason":  "error",
		"error":   owner + " plugin unavailable",
	}
	if _, _, err := st.appendBatch([]factIn{{
		Type: TypeTurnEnd,
		Role: RoleHost,
		Meta: mustJSON(meta),
	}}); err != nil {
		return false
	}
	p.clearStream(sessionID, turnID, "")
	return true
}

func (p *plugin) sessionFactContext(meta json.RawMessage) factContextMeta {
	var out factContextMeta
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &out)
	}
	return out
}
