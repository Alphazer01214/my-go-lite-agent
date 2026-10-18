package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
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

func TestStreamUpdateRejectsStaleSnapshots(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1"}); err != nil {
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

	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Name: "demo"}); err != nil {
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
	var chat chatStateUI
	if err := json.Unmarshal(ui.states["chat"][len(ui.states["chat"])-1], &chat); err != nil {
		t.Fatal(err)
	}
	if len(chat.Messages) != 1 || chat.Messages[0].Content != "final" {
		t.Fatalf("chat = %+v", chat)
	}
}

func TestStateReturnsHistoryChatAndTrace(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Name: "demo"}); err != nil {
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
	if _, err := p.create(context.Background(), createIn{SessionID: "s1"}); err != nil {
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
	if _, err := p.create(context.Background(), createIn{SessionID: "s1"}); err != nil {
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
	if _, err := p.create(context.Background(), createIn{SessionID: "s1"}); err != nil {
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
