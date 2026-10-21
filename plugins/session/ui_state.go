package main

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	// chatWindowMessages bounds every ChatState payload. Older messages are
	// fetched through session.messages when the user asks for them.
	chatWindowMessages = 200
	chatWindowBytes    = 2 << 20
	// traceWindowFacts bounds the audit window. session.facts serves older pages.
	traceWindowFacts = 200
	traceWindowBytes = 2 << 20
	// uiPageLimit caps one explicit page read.
	uiPageLimit = 500
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
	ToolName   string          `json:"tool_name,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
	Source     string          `json:"source,omitempty"`
	Kind       string          `json:"kind,omitempty"`
}

type chatTurnUI struct {
	TurnID          string `json:"turn_id,omitempty"`
	Status          string `json:"status"`
	Step            int    `json:"step,omitempty"`
	CancelRequested bool   `json:"cancel_requested"`
	Error           string `json:"error,omitempty"`
}

type chatTurnInfoUI struct {
	TurnID    string `json:"turn_id"`
	Status    string `json:"status"`
	Step      int    `json:"step,omitempty"`
	StartedAt int64  `json:"started_ts,omitempty"`
	EndedAt   int64  `json:"ended_ts,omitempty"`
	Messages  int    `json:"message_count"`
	Tools     int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

// chatUsageUI folds provider-reported llm_usage facts. It deliberately carries
// no local estimate: the dock meter must be the API's own accounting.
type chatUsageUI struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Requests         int    `json:"requests"`
	LastPromptTokens int    `json:"last_prompt_tokens"`
	Model            string `json:"model,omitempty"`
	ContextWindow    int    `json:"context_window,omitempty"`
}

// chatContextUI summarizes the newest context_snapshot fact. The inspector that
// expands it into messages lives in model-context.
type chatContextUI struct {
	ModelContextID string   `json:"model_context_id,omitempty"`
	TurnID         string   `json:"turn_id,omitempty"`
	StepID         string   `json:"step_id,omitempty"`
	MessageCount   int      `json:"message_count"`
	TokenEstimate  int      `json:"token_estimate"`
	Model          string   `json:"model,omitempty"`
	ContextWindow  int      `json:"context_window,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

type chatRequestUI struct {
	TurnID        string `json:"turn_id,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type chatStateUI struct {
	SessionID     string           `json:"session_id"`
	Name          string           `json:"name,omitempty"`
	WorkspaceID   string           `json:"workspace_id,omitempty"`
	Workspace     string           `json:"workspace,omitempty"`
	Model         string           `json:"model,omitempty"`
	ContextWindow int              `json:"context_window,omitempty"`
	Revision      int              `json:"revision"`
	ThroughSeq    int              `json:"through_seq"`
	FirstSeq      int              `json:"first_seq,omitempty"`
	HasMore       bool             `json:"has_more,omitempty"`
	Messages      []chatMessageUI  `json:"messages"`
	Turn          chatTurnUI       `json:"turn"`
	Turns         []chatTurnInfoUI `json:"turns"`
	Usage         chatUsageUI      `json:"usage"`
	Context       *chatContextUI   `json:"context,omitempty"`
	Request       *chatRequestUI   `json:"request,omitempty"`
	Stream        *streamSnapshot  `json:"stream,omitempty"`
	Error         string           `json:"error,omitempty"`
}

// chatAppendUI is the live delta for one append batch. AfterSeq is the last
// Fact sequence the receiver must already hold; through_seq covers host Facts
// that produce no message. A mismatch means the receiver must resync.
type chatAppendUI struct {
	SessionID  string            `json:"session_id"`
	Revision   int               `json:"revision"`
	AfterSeq   int               `json:"after_seq"`
	ThroughSeq int               `json:"through_seq"`
	Messages   []chatMessageUI   `json:"messages"`
	Turn       *chatTurnAppendUI `json:"turn,omitempty"`
}

// chatTurnAppendUI carries only fields the append can prove. It deliberately
// omits cancel_requested so a delta cannot clear an active cancellation.
type chatTurnAppendUI struct {
	TurnID string `json:"turn_id,omitempty"`
	Status string `json:"status,omitempty"`
	Step   int    `json:"step,omitempty"`
	Error  string `json:"error,omitempty"`
}

// streamDeltaUI is the live wire shape. The full draft stays in ChatState for
// resync; normal frames carry only text appended since the last frame.
type streamDeltaUI struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	StepID         string `json:"step_id"`
	UpdateID       int    `json:"update_id"`
	Reset          bool   `json:"reset"`
	ContentDelta   string `json:"content_delta,omitempty"`
	ReasoningDelta string `json:"reasoning_delta,omitempty"`
	Status         string `json:"status"`
}

// dockStateUI is the bottom bar projection. It never carries messages; the
// context inspector fetches those on demand.
type dockStateUI struct {
	SessionID     string         `json:"session_id,omitempty"`
	Name          string         `json:"name,omitempty"`
	WorkspaceID   string         `json:"workspace_id,omitempty"`
	Workspace     string         `json:"workspace,omitempty"`
	Revision      int            `json:"revision"`
	Model         string         `json:"model,omitempty"`
	ContextWindow int            `json:"context_window,omitempty"`
	Turn          chatTurnUI     `json:"turn"`
	Usage         chatUsageUI    `json:"usage"`
	Context       *chatContextUI `json:"context,omitempty"`
	Request       *chatRequestUI `json:"request,omitempty"`
}

type chatPanesStateUI struct {
	SelectedID  string        `json:"selected_id,omitempty"`
	WorkspaceID string        `json:"workspace_id,omitempty"`
	Revision    int           `json:"revision"`
	Panes       []chatStateUI `json:"panes"`
}

type historyStateUI struct {
	Sessions    []sessionSummary `json:"sessions"`
	SelectedID  string           `json:"selected_id,omitempty"`
	WorkspaceID string           `json:"workspace_id,omitempty"`
	// SelectedWorkspaceID is the registry's current workspace, which is not
	// necessarily WorkspaceID (that one follows the requested session). The
	// mount uses it to highlight the right workspace without a second call.
	SelectedWorkspaceID string            `json:"selected_workspace_id,omitempty"`
	Workspaces          []workspaceRecord `json:"workspaces"`
	OpenIDs             []string          `json:"open_ids,omitempty"`
	Revision            int               `json:"revision"`
}

type traceStateUI struct {
	SessionID  string `json:"session_id"`
	Revision   int    `json:"revision"`
	ThroughSeq int    `json:"through_seq"`
	FirstSeq   int    `json:"first_seq,omitempty"`
	HasMore    bool   `json:"has_more,omitempty"`
	Total      int    `json:"total"`
	Facts      []Fact `json:"facts"`
}

// traceAppendUI is the live delta for the audit view. Facts are the appended
// ones only; the client validates AfterSeq before merging.
type traceAppendUI struct {
	SessionID  string `json:"session_id"`
	Revision   int    `json:"revision"`
	AfterSeq   int    `json:"after_seq"`
	ThroughSeq int    `json:"through_seq"`
	Total      int    `json:"total"`
	Facts      []Fact `json:"facts"`
}

// dockAppendUI carries only dock fields that changed in one append batch.
// Usage is a delta; Context and Request are the newest whole summaries.
type dockAppendUI struct {
	SessionID string            `json:"session_id"`
	Revision  int               `json:"revision"`
	Turn      *chatTurnAppendUI `json:"turn,omitempty"`
	Usage     *chatUsageUI      `json:"usage,omitempty"`
	Context   *chatContextUI    `json:"context,omitempty"`
	Request   *chatRequestUI    `json:"request,omitempty"`
}

type stateIn struct {
	SessionID string `json:"session_id,omitempty"`
}

// messagesIn pages older Chat messages, ending before BeforeSeq (exclusive).
type messagesIn struct {
	SessionID string `json:"session_id"`
	BeforeSeq int    `json:"before_seq,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type messagesOut struct {
	SessionID string          `json:"session_id"`
	Messages  []chatMessageUI `json:"messages"`
	FirstSeq  int             `json:"first_seq,omitempty"`
	HasMore   bool            `json:"has_more"`
	Total     int             `json:"total"`
}

// factsIn pages older audit Facts, ending before BeforeSeq (exclusive).
type factsIn struct {
	SessionID string `json:"session_id"`
	BeforeSeq int    `json:"before_seq,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type factsOut struct {
	SessionID string `json:"session_id"`
	Facts     []Fact `json:"facts"`
	FirstSeq  int    `json:"first_seq,omitempty"`
	HasMore   bool   `json:"has_more"`
	Total     int    `json:"total"`
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
	Source    string `json:"source,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Error     string `json:"error,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
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
	reset := true
	contentDelta := in.Message.Content
	reasoningDelta := in.Message.Reasoning
	if ok && previous.TurnID == in.TurnID && previous.StepID == in.StepID &&
		strings.HasPrefix(in.Message.Content, previous.Message.Content) &&
		strings.HasPrefix(in.Message.Reasoning, previous.Message.Reasoning) {
		reset = false
		contentDelta = in.Message.Content[len(previous.Message.Content):]
		reasoningDelta = in.Message.Reasoning[len(previous.Message.Reasoning):]
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
	open := p.isOpenLocked(in.SessionID)
	p.mu.Unlock()

	// Every open pane tracks its own stream, including background sessions.
	// The outbox coalesces deltas; the handler never waits for a UI round trip.
	if open {
		p.enqueueStream(streamDeltaUI{
			SessionID: in.SessionID, TurnID: in.TurnID, StepID: in.StepID,
			UpdateID: in.UpdateID, Reset: reset,
			ContentDelta: contentDelta, ReasoningDelta: reasoningDelta, Status: in.Status,
		})
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
	p.dropStream(sessionID, turnID, stepID)
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
	trace := windowedTraceState(id, facts, p.nextUIRevision())
	out.Trace = &trace
	return out, nil
}

func lastFactSeq(facts []Fact) int {
	if len(facts) == 0 {
		return 0
	}
	return facts[len(facts)-1].Seq
}

func windowedTraceState(sessionID string, facts []Fact, revision int) traceStateUI {
	total := len(facts)
	start := total
	bytes := 0
	for start > 0 && total-start < traceWindowFacts {
		fact := facts[start-1]
		size := len(fact.Content) + len(fact.Meta) + len(fact.ToolCalls)
		if start < total && bytes+size > traceWindowBytes {
			break
		}
		bytes += size
		start--
	}
	window := facts[start:]
	state := traceStateUI{
		SessionID: sessionID, Revision: revision, ThroughSeq: lastFactSeq(facts),
		HasMore: start > 0, Total: total, Facts: window,
	}
	if len(window) > 0 {
		state.FirstSeq = window[0].Seq
	}
	return state
}

func windowChatMessages(messages []chatMessageUI) ([]chatMessageUI, int, bool) {
	total := len(messages)
	start := total
	bytes := 0
	for start > 0 && total-start < chatWindowMessages {
		message := messages[start-1]
		size := len(message.Content) + len(message.Reasoning) + len(message.ToolCalls)
		if start < total && bytes+size > chatWindowBytes {
			break
		}
		bytes += size
		start--
	}
	window := messages[start:]
	firstSeq := 0
	if len(window) > 0 {
		firstSeq = window[0].Seq
	}
	return window, firstSeq, start > 0
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

// publishAfterAppend reports count changes to History, then sends only the
// appended Facts to open panes. Full Chat/Trace State stays reserved for
// mount, selection, refresh and resync.
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

func chatMessageFromFact(fact Fact) chatMessageUI {
	var meta factContextMeta
	if len(fact.Meta) > 0 {
		_ = json.Unmarshal(fact.Meta, &meta)
	}
	return chatMessageUI{
		Seq: fact.Seq, TurnID: meta.TurnID, StepID: meta.StepID,
		Role: fact.Role, Content: fact.Content, Reasoning: meta.Reasoning,
		Timestamp: fact.Timestamp, ToolCalls: fact.ToolCalls, ToolCallID: fact.ToolCallID,
		ToolName: meta.ToolName, IsError: meta.IsError, Truncated: meta.Truncated,
		Source: meta.Source, Kind: meta.Kind,
	}
}

func turnAppendFromFacts(facts []Fact) *chatTurnAppendUI {
	var turn *chatTurnAppendUI
	for _, fact := range facts {
		var meta factContextMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		switch fact.Type {
		case TypeTurnStart:
			turn = &chatTurnAppendUI{TurnID: meta.TurnID, Status: "running"}
		case TypeStepStart:
			if turn == nil {
				turn = &chatTurnAppendUI{TurnID: meta.TurnID}
			}
			turn.TurnID = meta.TurnID
			turn.Status = "running"
			turn.Step = meta.Step
		case TypeTurnEnd:
			if turn == nil {
				turn = &chatTurnAppendUI{TurnID: meta.TurnID}
			}
			turn.TurnID = meta.TurnID
			turn.Status = meta.Reason
			turn.Error = meta.Error
		}
	}
	return turn
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
	state := chatAppendUI{
		SessionID: sessionID, Revision: p.nextUIRevision(), AfterSeq: afterSeq,
		ThroughSeq: throughSeq, Messages: messages, Turn: turnAppendFromFacts(facts),
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

func contextFromSnapshotMeta(meta json.RawMessage) *chatContextUI {
	var snap struct {
		ModelContextID string   `json:"model_context_id"`
		TurnID         string   `json:"turn_id"`
		StepID         string   `json:"step_id"`
		MessageCount   int      `json:"message_count"`
		TokenEstimate  int      `json:"token_estimate"`
		Model          string   `json:"model"`
		ContextWindow  int      `json:"context_window"`
		Warnings       []string `json:"warnings"`
	}
	_ = json.Unmarshal(meta, &snap)
	return &chatContextUI{
		ModelContextID: snap.ModelContextID,
		TurnID:         snap.TurnID,
		StepID:         snap.StepID,
		MessageCount:   snap.MessageCount,
		TokenEstimate:  snap.TokenEstimate,
		Model:          snap.Model,
		ContextWindow:  snap.ContextWindow,
		Warnings:       snap.Warnings,
	}
}

func requestFromHeaderMeta(meta json.RawMessage) *chatRequestUI {
	var rm struct {
		TurnID        string `json:"turn_id"`
		StepID        string `json:"step_id"`
		Mode          string `json:"mode"`
		Scheme        string `json:"scheme"`
		Model         string `json:"model"`
		ContextWindow int    `json:"context_window"`
	}
	_ = json.Unmarshal(meta, &rm)
	return &chatRequestUI{
		TurnID: rm.TurnID, StepID: rm.StepID,
		Mode: rm.Mode, Scheme: rm.Scheme,
		Model: rm.Model, ContextWindow: rm.ContextWindow,
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

func (p *plugin) publishDock(ctx context.Context, pane *chatStateUI) {
	state := dockStateUI{}
	if pane != nil {
		state = dockStateFromPane(*pane)
	}
	state.Revision = p.nextUIRevision()
	p.publishState(ctx, "dock", state)
}

// dockState is the resync read for the bottom mount. It resolves the effective
// session exactly like session.state, but returns only the dock projection.
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

// messages pages older Chat messages. BeforeSeq is exclusive; zero starts at
// the newest page. The returned slice stays in chronological order.
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

// facts pages older audit Facts. BeforeSeq is exclusive; zero starts at the
// newest page. The returned slice stays in chronological order.
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
		SessionID:     sessionID,
		Name:          meta.Name,
		WorkspaceID:   meta.WorkspaceID,
		Workspace:     meta.Workspace,
		Model:         meta.Model,
		ContextWindow: meta.ContextWindow,
		Messages:      make([]chatMessageUI, 0, len(facts)),
		Turn:          chatTurnUI{Status: "idle"},
	}
	var usage chatUsageUI
	var latestUsageModel string
	var latestUsageWindow int
	var contextMeta json.RawMessage
	var requestMeta json.RawMessage
	turnInfos := make(map[string]*chatTurnInfoUI)
	turnOrder := make([]string, 0)
	ensureTurn := func(id string) *chatTurnInfoUI {
		info, ok := turnInfos[id]
		if !ok {
			info = &chatTurnInfoUI{TurnID: id, Status: "running"}
			turnInfos[id] = info
			turnOrder = append(turnOrder, id)
		}
		return info
	}
	for _, fact := range facts {
		var meta factContextMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		switch fact.Type {
		case TypeMessage:
			if meta.Kind == "turn_cancelled" {
				break
			}
			state.Messages = append(state.Messages, chatMessageUI{
				Seq: fact.Seq, TurnID: meta.TurnID, StepID: meta.StepID,
				Role: fact.Role, Content: fact.Content, Reasoning: meta.Reasoning,
				Timestamp: fact.Timestamp, ToolCalls: fact.ToolCalls, ToolCallID: fact.ToolCallID,
				ToolName: meta.ToolName, IsError: meta.IsError, Truncated: meta.Truncated,
				Source: meta.Source, Kind: meta.Kind,
			})
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Messages++
				if fact.Role == RoleTool {
					info.Tools++
				}
			}
		case TypeTurnStart:
			state.Turn = chatTurnUI{TurnID: meta.TurnID, Status: "running"}
			turnOwner = meta.Plugin
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Status = "running"
				if info.StartedAt == 0 {
					info.StartedAt = fact.Timestamp
				}
			}
		case TypeStepStart:
			state.Turn.TurnID = meta.TurnID
			state.Turn.Step = meta.Step
			state.Turn.Status = "running"
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Status = "running"
				info.Step = meta.Step
				if info.StartedAt == 0 {
					info.StartedAt = fact.Timestamp
				}
			}
		case TypeTurnEnd:
			state.Turn.TurnID = meta.TurnID
			state.Turn.Status = meta.Reason
			state.Turn.Error = meta.Error
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Status = meta.Reason
				info.EndedAt = fact.Timestamp
				info.Error = meta.Error
			}
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
				latestUsageModel = um.Model
			}
			if um.ContextWindow > 0 {
				latestUsageWindow = um.ContextWindow
			}
		case TypeContextSnapshot:
			// Keep only the newest raw meta. A snapshot carries the full tool
			// list, so reparsing every historical snapshot would dominate a
			// long session's projection cost.
			contextMeta = fact.Meta
		case TypeRequestHeader:
			requestMeta = fact.Meta
		}
	}
	var contextSummary *chatContextUI
	if len(contextMeta) > 0 {
		var snap struct {
			ModelContextID string   `json:"model_context_id"`
			TurnID         string   `json:"turn_id"`
			StepID         string   `json:"step_id"`
			MessageCount   int      `json:"message_count"`
			TokenEstimate  int      `json:"token_estimate"`
			Model          string   `json:"model"`
			ContextWindow  int      `json:"context_window"`
			Warnings       []string `json:"warnings"`
		}
		_ = json.Unmarshal(contextMeta, &snap)
		contextSummary = &chatContextUI{
			ModelContextID: snap.ModelContextID,
			TurnID:         snap.TurnID,
			StepID:         snap.StepID,
			MessageCount:   snap.MessageCount,
			TokenEstimate:  snap.TokenEstimate,
			Model:          snap.Model,
			ContextWindow:  snap.ContextWindow,
			Warnings:       snap.Warnings,
		}
	}
	var requestSummary *chatRequestUI
	if len(requestMeta) > 0 {
		var rm struct {
			TurnID        string `json:"turn_id"`
			StepID        string `json:"step_id"`
			Mode          string `json:"mode"`
			Scheme        string `json:"scheme"`
			Model         string `json:"model"`
			ContextWindow int    `json:"context_window"`
		}
		_ = json.Unmarshal(requestMeta, &rm)
		requestSummary = &chatRequestUI{
			TurnID: rm.TurnID, StepID: rm.StepID,
			Mode: rm.Mode, Scheme: rm.Scheme,
			Model: rm.Model, ContextWindow: rm.ContextWindow,
		}
	}
	state.Turns = make([]chatTurnInfoUI, 0, len(turnOrder))
	for _, id := range turnOrder {
		state.Turns = append(state.Turns, *turnInfos[id])
	}
	usage.Model = latestUsageModel
	usage.ContextWindow = latestUsageWindow
	state.Usage = usage
	state.Context = contextSummary
	state.Request = requestSummary
	if state.Model == "" {
		state.Model = latestUsageModel
		if state.Model == "" && requestSummary != nil {
			state.Model = requestSummary.Model
		}
	}
	// The meter's numerator and denominator must come from the same request.
	// Session metadata is only a fallback before the first usage arrives.
	if latestUsageWindow > 0 {
		state.ContextWindow = latestUsageWindow
	}
	if state.ContextWindow <= 0 && contextSummary != nil {
		state.ContextWindow = contextSummary.ContextWindow
	}
	if state.ContextWindow <= 0 && requestSummary != nil {
		state.ContextWindow = requestSummary.ContextWindow
	}
	if state.Messages == nil {
		state.Messages = []chatMessageUI{}
	}
	state.Messages, state.FirstSeq, state.HasMore = windowChatMessages(state.Messages)
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
	state.ThroughSeq = lastFactSeq(facts)
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
