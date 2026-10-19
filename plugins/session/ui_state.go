package main

import (
	"context"
	"encoding/json"
	"strings"
)

type uiPublisher interface {
	SetState(mountID string, state json.RawMessage) error
	SendCommand(mountID string, command string, payload json.RawMessage) error
}

type chatMessageUI struct {
	Seq        int             `json:"seq"`
	TurnID     string          `json:"turn_id,omitempty"`
	StepID     string          `json:"step_id,omitempty"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Reasoning  string          `json:"reasoning,omitempty"`
	Timestamp  int64           `json:"timestamp,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type chatTurnUI struct {
	TurnID          string `json:"turn_id,omitempty"`
	Status          string `json:"status"`
	Step            int    `json:"step,omitempty"`
	CancelRequested bool   `json:"cancel_requested"`
}

type chatStateUI struct {
	SessionID   string          `json:"session_id"`
	Name        string          `json:"name,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	Revision    int             `json:"revision"`
	Messages    []chatMessageUI `json:"messages"`
	Turn        chatTurnUI      `json:"turn"`
	Stream      *streamSnapshot `json:"stream,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type chatPanesStateUI struct {
	SelectedID  string        `json:"selected_id,omitempty"`
	WorkspaceID string        `json:"workspace_id,omitempty"`
	Revision    int           `json:"revision"`
	Panes       []chatStateUI `json:"panes"`
}

type historyStateUI struct {
	Sessions    []sessionSummary  `json:"sessions"`
	SelectedID  string            `json:"selected_id,omitempty"`
	WorkspaceID string            `json:"workspace_id,omitempty"`
	// SelectedWorkspaceID is the registry's current workspace, which is not
	// necessarily WorkspaceID (that one follows the requested session). The
	// mount uses it to highlight the right workspace without a second call.
	SelectedWorkspaceID string            `json:"selected_workspace_id,omitempty"`
	Workspaces          []workspaceRecord `json:"workspaces"`
	OpenIDs             []string          `json:"open_ids,omitempty"`
	Revision            int               `json:"revision"`
}

type traceStateUI struct {
	SessionID string `json:"session_id"`
	Revision  int    `json:"revision"`
	Facts     []Fact `json:"facts"`
}

type stateIn struct {
	SessionID string `json:"session_id,omitempty"`
}

type stateOut struct {
	History historyStateUI `json:"history"`
	Chat    *chatStateUI   `json:"chat,omitempty"`
	Chats   []chatStateUI  `json:"chats,omitempty"`
	Trace   *traceStateUI  `json:"trace,omitempty"`
}

type selectIn struct {
	SessionID string `json:"session_id"`
}

type selectOut struct {
	SessionID string `json:"session_id"`
}

type closeIn struct {
	SessionID string `json:"session_id"`
}

type closeOut struct {
	SessionID string `json:"session_id"`
	Closed    bool   `json:"closed"`
	Selected  string `json:"selected_id,omitempty"`
}

type updateIn struct {
	SessionID   string  `json:"session_id"`
	Name        *string `json:"name,omitempty"`
	Workspace   *string `json:"workspace,omitempty"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
}

type updateOut struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

type factContextMeta struct {
	TurnID    string `json:"turn_id"`
	StepID    string `json:"step_id"`
	Reason    string `json:"reason"`
	Reasoning string `json:"reasoning"`
	Step      int    `json:"step"`
	Plugin    string `json:"plugin,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (p *plugin) setUIPublisher(ui uiPublisher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ui = ui
}

func (p *plugin) updateStream(ctx context.Context, in streamUpdateIn) (streamUpdateOut, error) {
	if in.SessionID == "" || in.TurnID == "" || in.StepID == "" {
		return streamUpdateOut{}, errBadArguments("session_id, turn_id and step_id are required")
	}
	if in.UpdateID <= 0 {
		return streamUpdateOut{}, errBadArguments("update_id must be positive")
	}
	if in.Status == "" {
		in.Status = "streaming"
	}
	if _, err := p.get(in.SessionID); err != nil {
		return streamUpdateOut{}, err
	}

	p.mu.Lock()
	if p.streams == nil {
		p.streams = make(map[string]streamSnapshot)
	}
	previous, ok := p.streams[in.SessionID]
	if ok && previous.TurnID == in.TurnID && previous.StepID == in.StepID && in.UpdateID <= previous.UpdateID {
		p.mu.Unlock()
		return streamUpdateOut{Accepted: false, Reason: "stale"}, nil
	}
	snapshot := streamSnapshot{
		SessionID: in.SessionID,
		TurnID:    in.TurnID,
		StepID:    in.StepID,
		UpdateID:  in.UpdateID,
		Message:   in.Message,
		Status:    in.Status,
	}
	p.streams[in.SessionID] = snapshot
	ui := p.ui
	open := p.isOpenLocked(in.SessionID)
	p.mu.Unlock()

	// Every open pane tracks its own stream, including background sessions.
	if ui != nil && open {
		payload, _ := json.Marshal(snapshot)
		_ = ui.SendCommand("chat", "stream", payload)
	}
	if in.Status == "cancel_requested" {
		p.publishPanes(ctx)
	}
	return streamUpdateOut{Accepted: true}, nil
}

func (p *plugin) clearStream(sessionID, turnID, stepID string) {
	p.mu.Lock()
	if snapshot, ok := p.streams[sessionID]; ok &&
		(turnID == "" || snapshot.TurnID == turnID) &&
		(stepID == "" || snapshot.StepID == stepID) {
		delete(p.streams, sessionID)
	}
	p.mu.Unlock()
}

// selectedID is the Runtime-owned selection. The browser never owns it.
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

// selectSession moves the selection and republishes every mount that depends on it.
func (p *plugin) selectSession(ctx context.Context, in selectIn) (selectOut, error) {
	if in.SessionID == "" {
		return selectOut{}, errBadArguments("session_id is required")
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return selectOut{}, err
	}
	meta := st.snapshotMeta()
	if err := p.selectWorkspace(meta.WorkspaceID); err != nil {
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
		SessionID: in.SessionID, Name: meta.Name, Workspace: meta.Workspace,
	}, nil
}

// effectiveID resolves the session a state read or selection-free call applies to.
// The open selection wins: it is the session on screen, and a registry
// workspace switch must not flip the view until the user picks another session.
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
	list, err := p.list(workspaceID)
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
				_ = p.selectWorkspace(workspaceID)
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
	list, err := p.list(workspaceID)
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
	out.Trace = &traceStateUI{SessionID: id, Revision: p.nextUIRevision(), Facts: facts}
	return out, nil
}

// publishForSelection pushes History plus the selected session's Chat and Trace.
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

// publishAfterAppend reports count changes to History, and refreshes the
// visible Chat and Trace only when the appended session is the selected one.
func (p *plugin) publishAfterAppend(ctx context.Context, sessionID string) {
	p.publishHistory(ctx)
	if p.isOpen(sessionID) {
		p.publishPanes(ctx)
	}
	if sessionID == p.selectedID() {
		p.publishTrace(ctx, sessionID)
	}
}

// publishAfterCreate never steals an existing selection. When nothing is
// selected yet, the Runtime adopts the new session so the first message from
// the browser becomes visible without a manual pick.
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

// uiPaneStates renders the panes to push: every open pane of the workspace
// plus the session on screen when it belongs to another workspace, so a
// registry workspace switch never blanks the pane the user is looking at.
// One unreadable pane is skipped rather than failing the batch: a stale pane
// must not blank the rest.
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
	found := false
	for _, pane := range panes {
		if pane.SessionID == selected {
			found = true
			break
		}
	}
	if !found {
		selected = ""
	}
	state := chatPanesStateUI{
		SelectedID:  selected,
		WorkspaceID: workspaceID,
		Revision:    p.nextUIRevision(),
		Panes:       panes,
	}
	p.publishState(ctx, "chat", state)
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
	out, err := p.list(workspaceID)
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
	state := traceStateUI{SessionID: sessionID, Revision: p.nextUIRevision(), Facts: facts}
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

func (p *plugin) chatState(ctx context.Context, sessionID string) (chatStateUI, error) {
	return p.chatStateInternal(ctx, sessionID, true)
}

func (p *plugin) chatStateInternal(ctx context.Context, sessionID string, allowRecovery bool) (chatStateUI, error) {
	st, err := p.get(sessionID)
	if err != nil {
		return chatStateUI{}, err
	}
	meta := st.snapshotMeta()
	facts, _, err := p.readFacts(sessionID)
	if err != nil {
		return chatStateUI{}, err
	}
	turnOwner := ""
	state := chatStateUI{
		SessionID:   sessionID,
		Name:        meta.Name,
		WorkspaceID: meta.WorkspaceID,
		Workspace:   meta.Workspace,
		Messages:    make([]chatMessageUI, 0, len(facts)),
		Turn:        chatTurnUI{Status: "idle"},
	}
	for _, fact := range facts {
		var meta factContextMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		switch fact.Type {
		case TypeMessage:
			state.Messages = append(state.Messages, chatMessageUI{
				Seq: fact.Seq, TurnID: meta.TurnID, StepID: meta.StepID,
				Role: fact.Role, Content: fact.Content, Reasoning: meta.Reasoning,
				Timestamp: fact.Timestamp, ToolCalls: fact.ToolCalls, ToolCallID: fact.ToolCallID,
			})
		case TypeTurnStart:
			state.Turn = chatTurnUI{TurnID: meta.TurnID, Status: "running"}
			turnOwner = meta.Plugin
		case TypeStepStart:
			state.Turn.TurnID = meta.TurnID
			state.Turn.Step = meta.Step
			state.Turn.Status = "running"
		case TypeTurnEnd:
			state.Turn.TurnID = meta.TurnID
			state.Turn.Status = meta.Reason
		}
	}
	if state.Messages == nil {
		state.Messages = []chatMessageUI{}
	}
	ownerState := p.pluginState(ctx, turnOwner)
	if allowRecovery && state.Turn.Status == "running" && turnOwner != "" && turnOwnerUnavailable(ownerState) {
		if p.recoverOpenTurn(sessionID, state.Turn.TurnID, turnOwner) {
			return p.chatStateInternal(ctx, sessionID, false)
		}
	}
	p.mu.RLock()
	stream, streamOK := p.streams[sessionID]
	p.mu.RUnlock()
	if streamOK {
		copy := stream
		state.Stream = &copy
	}
	if streamOK && stream.Status == "cancel_requested" && stream.TurnID == state.Turn.TurnID {
		state.Turn.CancelRequested = true
	}
	state.Revision = p.nextUIRevision()
	return state, nil
}

func turnOwnerUnavailable(state string) bool {
	switch state {
	case "failed", "inactive", "stopping", "missing":
		return true
	default:
		return false
	}
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
func (p *plugin) nextUIRevision() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.uiRevision++
	return int(p.uiRevision)
}

func (p *plugin) sessionFactContext(meta json.RawMessage) factContextMeta {
	var out factContextMeta
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &out)
	}
	return out
}
