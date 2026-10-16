package main

import (
	"encoding/json"
)

type uiPublisher interface {
	SetState(mountID string, state json.RawMessage) error
	SendCommand(mountID string, command string, payload json.RawMessage) error
}

type streamUpdateIn struct {
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	StepID    string        `json:"step_id"`
	UpdateID  int           `json:"update_id"`
	Message   streamMessage `json:"message"`
	Status    string        `json:"status"`
}

type streamUpdateOut struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

type streamMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Reasoning  string          `json:"reasoning,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type streamSnapshot struct {
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	StepID    string        `json:"step_id"`
	UpdateID  int           `json:"update_id"`
	Message   streamMessage `json:"message"`
	Status    string        `json:"status"`
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
	SessionID string          `json:"session_id"`
	Revision  int             `json:"revision"`
	Messages  []chatMessageUI `json:"messages"`
	Turn      chatTurnUI      `json:"turn"`
	Error     string          `json:"error,omitempty"`
}

type historyStateUI struct {
	Sessions   []sessionSummary `json:"sessions"`
	SelectedID string           `json:"selected_id,omitempty"`
	Revision   int              `json:"revision"`
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
	Trace   *traceStateUI  `json:"trace,omitempty"`
}

type selectIn struct {
	SessionID string `json:"session_id"`
}

type selectOut struct {
	SessionID string `json:"session_id"`
}

type factContextMeta struct {
	TurnID    string `json:"turn_id"`
	StepID    string `json:"step_id"`
	Reason    string `json:"reason"`
	Reasoning string `json:"reasoning"`
	Step      int    `json:"step"`
}

func (p *plugin) setUIPublisher(ui uiPublisher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ui = ui
}

func (p *plugin) updateStream(in streamUpdateIn) (streamUpdateOut, error) {
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
	selected := p.selected
	p.mu.Unlock()

	// A stream only belongs to the component showing that session.
	if ui != nil && selected == in.SessionID {
		payload, _ := json.Marshal(snapshot)
		_ = ui.SendCommand("chat", "stream", payload)
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

// selectSession moves the selection and republishes every mount that depends on it.
func (p *plugin) selectSession(in selectIn) (selectOut, error) {
	if in.SessionID == "" {
		return selectOut{}, errBadArguments("session_id is required")
	}
	if _, err := p.get(in.SessionID); err != nil {
		return selectOut{}, err
	}
	p.setSelected(in.SessionID)
	p.publishForSelection()
	return selectOut{SessionID: in.SessionID}, nil
}

// effectiveID resolves the session a state read or selection-free call applies to.
func (p *plugin) effectiveID(requested string) string {
	if requested != "" {
		return requested
	}
	if id := p.selectedID(); id != "" {
		return id
	}
	list, err := p.list()
	if err != nil || len(list.Sessions) == 0 {
		return ""
	}
	id := list.Sessions[0].SessionID
	p.setSelected(id)
	return id
}

func (p *plugin) state(in stateIn) (stateOut, error) {
	id := p.effectiveID(in.SessionID)
	out := stateOut{History: historyStateUI{Sessions: []sessionSummary{}, SelectedID: id}}
	list, err := p.list()
	if err != nil {
		return stateOut{}, err
	}
	out.History.Sessions = list.Sessions
	out.History.Revision = p.nextUIRevision()
	if id == "" {
		return out, nil
	}
	chat, err := p.chatState(id)
	if err != nil {
		return stateOut{}, err
	}
	st, err := p.get(id)
	if err != nil {
		return stateOut{}, err
	}
	facts, _, err := p.readFacts(id)
	if err != nil {
		return stateOut{}, err
	}
	out.Chat = &chat
	out.Trace = &traceStateUI{SessionID: id, Revision: st.nextSeq, Facts: facts}
	return out, nil
}

// publishForSelection pushes History plus the selected session's Chat and Trace.
func (p *plugin) publishForSelection() {
	p.publishHistory()
	p.publishSession(p.selectedID())
}

func (p *plugin) publishSession(sessionID string) {
	if sessionID == "" {
		return
	}
	p.publishChat(sessionID)
	p.publishTrace(sessionID)
}

// publishAfterAppend reports count changes to History, and refreshes the
// visible Chat and Trace only when the appended session is the selected one.
func (p *plugin) publishAfterAppend(sessionID string) {
	p.publishHistory()
	if sessionID == p.selectedID() {
		p.publishSession(sessionID)
	}
}

// publishAfterCreate never steals an existing selection. When nothing is
// selected yet, the Runtime adopts the new session so the first message from
// the browser becomes visible without a manual pick.
func (p *plugin) publishAfterCreate(sessionID string) {
	if p.selectedID() == "" {
		p.setSelected(sessionID)
		p.publishForSelection()
		return
	}
	p.publishHistory()
}

func (p *plugin) publishChat(sessionID string) {
	state, err := p.chatState(sessionID)
	if err != nil {
		return
	}
	p.publishState("chat", state)
}

func (p *plugin) publishHistory() {
	p.mu.RLock()
	ui := p.ui
	p.mu.RUnlock()
	if ui == nil {
		return
	}
	out, err := p.list()
	if err != nil {
		return
	}
	state := historyStateUI{Sessions: out.Sessions, SelectedID: p.selectedID(), Revision: p.nextUIRevision()}
	p.publishState("history", state)
}

func (p *plugin) publishTrace(sessionID string) {
	if _, err := p.get(sessionID); err != nil {
		return
	}
	facts, _, err := p.readFacts(sessionID)
	if err != nil {
		return
	}
	state := traceStateUI{SessionID: sessionID, Revision: p.nextUIRevision(), Facts: facts}
	p.publishState("trace", state)
}

func (p *plugin) publishState(mountID string, state any) {
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

func (p *plugin) chatState(sessionID string) (chatStateUI, error) {
	if _, err := p.get(sessionID); err != nil {
		return chatStateUI{}, err
	}
	facts, _, err := p.readFacts(sessionID)
	if err != nil {
		return chatStateUI{}, err
	}
	state := chatStateUI{
		SessionID: sessionID,
		Messages:  make([]chatMessageUI, 0, len(facts)),
		Turn:      chatTurnUI{Status: "idle"},
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
	state.Revision = p.nextUIRevision()
	return state, nil
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
