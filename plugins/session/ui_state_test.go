package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type fakeUIPublisher struct {
	mu       sync.Mutex
	states   map[string][]json.RawMessage
	commands map[string][]json.RawMessage
}

func newFakeUIPublisher() *fakeUIPublisher {
	return &fakeUIPublisher{
		states:   make(map[string][]json.RawMessage),
		commands: make(map[string][]json.RawMessage),
	}
}

func (f *fakeUIPublisher) SetState(mountID string, state json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[mountID] = append(f.states[mountID], append(json.RawMessage(nil), state...))
	return nil
}

func (f *fakeUIPublisher) SendCommand(mountID, command string, payload json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands[command] = append(f.commands[command], append(json.RawMessage(nil), payload...))
	return nil
}

func TestUIRevisionAdvancesAcrossRestart(t *testing.T) {
	first := newPlugin(t.TempDir())
	before := first.nextUIRevision()
	first.close()
	time.Sleep(time.Millisecond)

	second := newPlugin(t.TempDir())
	after := second.nextUIRevision()
	second.close()
	if after <= before {
		t.Fatalf("revision after restart = %d, want > %d", after, before)
	}
}

func TestStreamUpdateRejectsStaleSnapshots(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	base := streamUpdateIn{
		SessionID: "s1", TurnID: "t1", StepID: "st1", UpdateID: 2,
		Message: streamMessage{Role: RoleAssistant, Content: "new"},
		Status:  "streaming",
	}
	if out, err := p.updateStream(context.Background(), base); err != nil || !out.Accepted {
		t.Fatalf("first update = %+v, %v", out, err)
	}
	base.UpdateID = 1
	base.Message.Content = "old"
	out, err := p.updateStream(context.Background(), base)
	if err != nil || out.Accepted || out.Reason != "stale" {
		t.Fatalf("stale update = %+v, %v", out, err)
	}

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 1 {
		t.Fatalf("commands = %#v", ui.commands)
	}
	var snapshot streamSnapshot
	if err := json.Unmarshal(ui.commands["stream"][0], &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Message.Content != "new" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestAppendFinalAssistantClearsStreamAndPublishesStates(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)

	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Name: "demo", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	_, err := p.updateStream(context.Background(), streamUpdateIn{
		SessionID: "s1", TurnID: "t1", StepID: "st1", UpdateID: 1,
		Message: streamMessage{Role: RoleAssistant, Content: "draft"},
		Status:  "streaming",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.append(context.Background(), appendIn{
		SessionID: "s1",
		Facts: []factIn{{
			Type:    TypeMessage,
			Role:    RoleAssistant,
			Content: "final",
			Meta:    mustJSON(map[string]any{"turn_id": "t1", "step_id": "st1"}),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	p.mu.RLock()
	_, exists := p.streams["s1"]
	p.mu.RUnlock()
	if exists {
		t.Fatal("stream state was not cleared")
	}

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.states["chat"]) == 0 || len(ui.states["history"]) == 0 || len(ui.states["trace"]) == 0 {
		t.Fatalf("states = %#v", ui.states)
	}
	var panes chatPanesStateUI
	if err := json.Unmarshal(ui.states["chat"][len(ui.states["chat"])-1], &panes); err != nil {
		t.Fatal(err)
	}
	if len(panes.Panes) != 1 || len(panes.Panes[0].Messages) != 1 || panes.Panes[0].Messages[0].Content != "final" {
		t.Fatalf("panes = %+v", panes)
	}
}

func TestSelectOpensMultiplePanesAndCloseSelectsNeighbor(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	workspace := t.TempDir()
	for _, id := range []string{"s1", "s2"} {
		if _, err := p.create(context.Background(), createIn{SessionID: id, Workspace: workspace}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s2"}); err != nil {
		t.Fatal(err)
	}
	if got := p.openIDs(); len(got) != 2 || got[0] != "s1" || got[1] != "s2" {
		t.Fatalf("open ids = %v", got)
	}
	out, err := p.closeSession(context.Background(), closeIn{SessionID: "s2"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Closed || out.Selected != "s1" {
		t.Fatalf("close = %+v", out)
	}
	state, err := p.state(context.Background(), stateIn{})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Chats) != 1 || state.Chats[0].SessionID != "s1" || state.Chat == nil || state.Chat.SessionID != "s1" {
		t.Fatalf("state = %+v", state)
	}
}

func TestUpdateNamePersistsAndKeepsWorkspaceImmutable(t *testing.T) {
	dir := t.TempDir()
	workspace := t.TempDir()
	p := newPlugin(dir)
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: workspace}); err != nil {
		t.Fatal(err)
	}
	out, err := p.updateSession(context.Background(), updateIn{
		SessionID: "s1", Name: stringPtr("project"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "project" || !sameWorkspacePath(out.Workspace, workspace) {
		t.Fatalf("update = %+v", out)
	}
	chat, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if chat.Name != "project" || !sameWorkspacePath(chat.Workspace, workspace) {
		t.Fatalf("chat = %+v", chat)
	}
	before, _, err := p.readFacts("s1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.updateSession(context.Background(), updateIn{
		SessionID: "s1", Workspace: stringPtr(t.TempDir()),
	}); err == nil {
		t.Fatal("workspace update was accepted")
	}
	if _, err := p.updateSession(context.Background(), updateIn{
		SessionID: "s1", Name: stringPtr("project"),
	}); err != nil {
		t.Fatal(err)
	}
	after, _, err := p.readFacts("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("no-op update appended facts: %d -> %d", len(before), len(after))
	}
	reloaded := newPlugin(dir)
	t.Cleanup(reloaded.close)
	reloaded.loadAll()
	info, err := reloaded.info(infoIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "project" || !sameWorkspacePath(info.Workspace, workspace) {
		t.Fatalf("reloaded info = %+v", info)
	}
}

func TestStreamCommandReachesOpenBackgroundPane(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	workspace := t.TempDir()
	for _, id := range []string{"s1", "s2"} {
		if _, err := p.create(context.Background(), createIn{SessionID: id, Workspace: workspace}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.updateStream(context.Background(), streamUpdateIn{
		SessionID: "s1", TurnID: "t1", StepID: "st1", UpdateID: 1,
		Message: streamMessage{Role: RoleAssistant, Content: "background"},
	}); err != nil {
		t.Fatal(err)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 1 {
		t.Fatalf("stream commands = %#v", ui.commands)
	}
	var snapshot streamSnapshot
	if err := json.Unmarshal(ui.commands["stream"][0], &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SessionID != "s1" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func stringPtr(value string) *string { return &value }

func TestStateReturnsHistoryChatAndTrace(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Name: "demo", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{
		SessionID: "s1",
		Facts: []factIn{{
			Type:    TypeMessage,
			Role:    RoleUser,
			Content: "hello",
			Meta:    mustJSON(map[string]any{"turn_id": "t1"}),
		}},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.History.Sessions) != 1 || out.Chat == nil || out.Trace == nil {
		t.Fatalf("state = %+v", out)
	}
	if len(out.Chat.Messages) != 1 || out.Chat.Messages[0].Content != "hello" {
		t.Fatalf("chat = %+v", out.Chat)
	}
	if len(out.Trace.Facts) < 2 {
		t.Fatalf("trace = %+v", out.Trace)
	}
}

func TestStateRevisionAdvancesAndCarriesReasoning(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{
		SessionID: "s1",
		Facts: []factIn{{
			Type:    TypeMessage,
			Role:    RoleAssistant,
			Content: "answer",
			Meta: mustJSON(map[string]any{
				"turn_id": "t1", "step_id": "st1", "reasoning": "internal note",
			}),
		}},
	}); err != nil {
		t.Fatal(err)
	}

	first, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Chat.Revision <= first.Chat.Revision {
		t.Fatalf("chat revisions = %d then %d", first.Chat.Revision, second.Chat.Revision)
	}
	if len(first.Chat.Messages) != 1 || first.Chat.Messages[0].Reasoning != "internal note" ||
		first.Chat.Messages[0].Timestamp <= 0 {
		t.Fatalf("message = %#v", first.Chat.Messages)
	}
}

func TestCancelRequestedPublishesChatState(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeTurnStart, Role: RoleHost, Meta: mustJSON(map[string]any{"turn_id": "t1"}),
	}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := p.updateStream(context.Background(), streamUpdateIn{
		SessionID: "s1", TurnID: "t1", StepID: "st1", UpdateID: 1,
		Message: streamMessage{Role: RoleAssistant},
		Status:  "cancel_requested",
	}); err != nil {
		t.Fatal(err)
	}

	state, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Turn.CancelRequested {
		t.Fatalf("chat state = %+v", state)
	}
}

func TestStateTraceRevisionUsesUIRevision(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	first, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Trace.Revision <= first.Trace.Revision {
		t.Fatalf("trace revisions = %d then %d", first.Trace.Revision, second.Trace.Revision)
	}
}
