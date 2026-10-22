package main

import (
	"context"
	"encoding/json"
)

func (p *plugin) publishForSelection(ctx context.Context) {
	p.publishHistory(ctx)
	p.publishPanes(ctx)
	p.publishTraceSelected(ctx)
}

func (p *plugin) publishSession(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	p.publishPanes(ctx)
	p.publishTrace(ctx, sessionID)
}

func (p *plugin) publishAfterAppend(ctx context.Context, sessionID string, facts []Fact, afterSeq, throughSeq int) {
	p.publishHistory(ctx)
	if p.isOpen(sessionID) {
		p.publishChatAppend(ctx, sessionID, facts, afterSeq, throughSeq)
	}
	if sessionID == p.selectedID() {
		p.publishTraceAppend(ctx, sessionID, facts, afterSeq, throughSeq)
		p.publishDockAppend(ctx, sessionID, facts)
	}
}

func (p *plugin) publishChatAppend(ctx context.Context, sessionID string, facts []Fact, afterSeq, throughSeq int) {
	messages := make([]chatMessageUI, 0, len(facts))
	for _, fact := range facts {
		if fact.Type != TypeMessage {
			continue
		}
		var meta factContextMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		if meta.Kind == "turn_cancelled" {
			continue
		}
		messages = append(messages, chatMessageFromFact(fact))
	}
	var subagents []subagentUI
	for _, fact := range facts {
		if fact.Type == TypeSubagentSpawned || fact.Type == TypeSubagentFinished {
			subagents = p.subagentsForSession(sessionID)
			break
		}
	}
	state := chatAppendUI{
		SessionID: sessionID, Revision: p.nextUIRevision(), AfterSeq: afterSeq,
		ThroughSeq: throughSeq, Messages: messages, Turn: turnAppendFromFacts(facts),
		Subagents: subagents,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}
	p.mu.RLock()
	ui := p.ui
	p.mu.RUnlock()
	if ui == nil {
		return
	}
	_ = ui.SendCommand("chat", "append", payload)
}

func (p *plugin) publishTraceAppend(ctx context.Context, sessionID string, facts []Fact, afterSeq, throughSeq int) {
	total := len(facts)
	if st, err := p.get(sessionID); err == nil {
		total, _ = st.counts()
	}
	state := traceAppendUI{
		SessionID: sessionID, Revision: p.nextUIRevision(), AfterSeq: afterSeq,
		ThroughSeq: throughSeq, Total: total, Facts: facts,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}
	p.mu.RLock()
	ui := p.ui
	p.mu.RUnlock()
	if ui == nil {
		return
	}
	_ = ui.SendCommand("trace", "append", payload)
}

func (p *plugin) publishDockAppend(ctx context.Context, sessionID string, facts []Fact) {
	state := dockAppendUI{SessionID: sessionID, Revision: p.nextUIRevision(), Turn: turnAppendFromFacts(facts)}
	var usage chatUsageUI
	usageSeen := false
	var contextMeta, requestMeta json.RawMessage
	for _, fact := range facts {
		switch fact.Type {
		case TypeLLMUsage:
			var um struct {
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
				Model         string `json:"model"`
				ContextWindow int    `json:"context_window"`
			}
			if len(fact.Meta) > 0 {
				_ = json.Unmarshal(fact.Meta, &um)
			}
			total := um.Usage.TotalTokens
			if total == 0 {
				total = um.Usage.PromptTokens + um.Usage.CompletionTokens
			}
			usage.PromptTokens += um.Usage.PromptTokens
			usage.CompletionTokens += um.Usage.CompletionTokens
			usage.TotalTokens += total
			usage.Requests++
			usage.LastPromptTokens = um.Usage.PromptTokens
			if um.Model != "" {
				usage.Model = um.Model
			}
			if um.ContextWindow > 0 {
				usage.ContextWindow = um.ContextWindow
			}
			usageSeen = true
		case TypeContextSnapshot:
			contextMeta = fact.Meta
		case TypeRequestHeader:
			requestMeta = fact.Meta
		}
	}
	if usageSeen {
		state.Usage = &usage
	}
	if len(contextMeta) > 0 {
		state.Context = contextFromSnapshotMeta(contextMeta)
	}
	if len(requestMeta) > 0 {
		state.Request = requestFromHeaderMeta(requestMeta)
	}
	if state.Turn == nil && state.Usage == nil && state.Context == nil && state.Request == nil {
		return
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}
	p.mu.RLock()
	ui := p.ui
	p.mu.RUnlock()
	if ui == nil {
		return
	}
	_ = ui.SendCommand("dock", "append", payload)
}

func (p *plugin) publishAfterCreate(ctx context.Context, sessionID string) {
	st, err := p.get(sessionID)
	if err != nil {
		p.publishHistory(ctx)
		return
	}
	meta := st.snapshotMeta()
	current, hasCurrent := p.selectedWorkspace()
	if hasCurrent && meta.WorkspaceID != "" && meta.WorkspaceID != current.WorkspaceID {
		p.publishHistory(ctx)
		return
	}
	p.mu.Lock()
	alreadySelected := p.selectedByWorkspace[meta.WorkspaceID] != ""
	p.openSessionLocked(sessionID)
	if !alreadySelected {
		p.selected = sessionID
		p.selectedByWorkspace[meta.WorkspaceID] = sessionID
	}
	p.mu.Unlock()
	p.publishForSelection(ctx)
}

func (p *plugin) uiPaneStates(ctx context.Context, workspaceID, onScreen string) []chatStateUI {
	p.mu.RLock()
	ids := make([]string, 0, len(p.open))
	for _, id := range p.open {
		st := p.reg[id]
		if st == nil {
			continue
		}
		if workspaceID == "" || id == onScreen || st.snapshotMeta().WorkspaceID == workspaceID {
			ids = append(ids, id)
		}
	}
	p.mu.RUnlock()
	panes := make([]chatStateUI, 0, len(ids))
	for _, id := range ids {
		state, err := p.chatState(ctx, id)
		if err != nil {
			continue
		}
		panes = append(panes, state)
	}
	return panes
}

func (p *plugin) publishPanes(ctx context.Context) {
	workspaceID := ""
	if current, ok := p.selectedWorkspace(); ok {
		workspaceID = current.WorkspaceID
	}
	selected := p.selectedID()
	panes := p.uiPaneStates(ctx, workspaceID, selected)
	// SelectedID must live inside the pushed set: the browser keys its view
	// off it, and a dangling id makes the pane fall back to another session.
	var selectedPane *chatStateUI
	for i := range panes {
		if panes[i].SessionID == selected {
			selectedPane = &panes[i]
			break
		}
	}
	if selectedPane == nil {
		selected = ""
	}
	state := chatPanesStateUI{
		SelectedID:  selected,
		WorkspaceID: workspaceID,
		Revision:    p.nextUIRevision(),
		Panes:       panes,
	}
	p.publishState(ctx, "chat", state)
	p.publishDock(ctx, selectedPane)
}

func (p *plugin) publishDock(ctx context.Context, pane *chatStateUI) {
	state := dockStateUI{}
	if pane != nil {
		state = dockStateFromPane(*pane)
	}
	state.Revision = p.nextUIRevision()
	p.publishState(ctx, "dock", state)
}

func (p *plugin) publishTraceSelected(ctx context.Context) {
	id := p.selectedID()
	if id == "" {
		p.publishState(ctx, "trace", traceStateUI{
			Revision: p.nextUIRevision(),
			Facts:    []Fact{},
		})
		return
	}
	p.publishTrace(ctx, id)
}

func (p *plugin) publishHistory(ctx context.Context) {
	p.mu.RLock()
	ui := p.ui
	p.mu.RUnlock()
	if ui == nil {
		return
	}
	workspaceID := ""
	workspaces, selectedWorkspaceID := p.workspaceRecords()
	if current, ok := p.selectedWorkspace(); ok {
		workspaceID = current.WorkspaceID
	}
	out, err := p.list(workspaceID, false)
	if err != nil {
		return
	}
	state := historyStateUI{
		Sessions: out.Sessions, SelectedID: p.selectedID(),
		WorkspaceID: workspaceID, SelectedWorkspaceID: selectedWorkspaceID,
		Workspaces: workspaces,
		OpenIDs:    p.openIDsFor(workspaceID), Revision: p.nextUIRevision(),
	}
	p.publishState(ctx, "history", state)
}

func (p *plugin) publishTrace(ctx context.Context, sessionID string) {
	if _, err := p.get(sessionID); err != nil {
		return
	}
	facts, _, err := p.readFacts(sessionID)
	if err != nil {
		return
	}
	state := windowedTraceState(sessionID, facts, p.nextUIRevision())
	p.publishState(ctx, "trace", state)
}

func (p *plugin) publishState(ctx context.Context, mountID string, state any) {
	p.mu.RLock()
	ui := p.ui
	p.mu.RUnlock()
	if ui == nil {
		return
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = ui.SetState(mountID, payload)
}

func (p *plugin) nextUIRevision() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.uiRevision++
	return int(p.uiRevision)
}

func (p *plugin) setUIPublisher(ui uiPublisher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ui = ui
}
