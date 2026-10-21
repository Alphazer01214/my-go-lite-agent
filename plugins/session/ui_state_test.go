package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeUIPublisher struct {
	mu       sync.Mutex
	states   map[string][]json.RawMessage
	commands map[string][]json.RawMessage
	byMount  map[string][]json.RawMessage
}

func newFakeUIPublisher() *fakeUIPublisher {
	return &fakeUIPublisher{
		states:   make(map[string][]json.RawMessage),
		commands: make(map[string][]json.RawMessage),
		byMount:  make(map[string][]json.RawMessage),
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
	copied := append(json.RawMessage(nil), payload...)
	f.commands[command] = append(f.commands[command], copied)
	key := mountID + ":" + command
	f.byMount[key] = append(f.byMount[key], copied)
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
	p.flushStreamOutbox()

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 1 {
		t.Fatalf("commands = %#v", ui.commands)
	}
	var delta streamDeltaUI
	if err := json.Unmarshal(ui.commands["stream"][0], &delta); err != nil {
		t.Fatal(err)
	}
	if !delta.Reset || delta.ContentDelta != "new" || delta.UpdateID != 2 {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestStreamUpdateSendsOnlyAppendedText(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	first := streamUpdateIn{
		SessionID: "s1", TurnID: "t1", StepID: "st1", UpdateID: 1,
		Message: streamMessage{Role: RoleAssistant, Content: "hello", Reasoning: "why"},
		Status:  "streaming",
	}
	if _, err := p.updateStream(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	p.flushStreamOutbox()
	second := first
	second.UpdateID = 2
	second.Message.Content = "hello world"
	second.Message.Reasoning = "why not"
	if _, err := p.updateStream(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	p.flushStreamOutbox()

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 2 {
		t.Fatalf("stream commands = %d", len(ui.commands["stream"]))
	}
	var delta streamDeltaUI
	if err := json.Unmarshal(ui.commands["stream"][1], &delta); err != nil {
		t.Fatal(err)
	}
	if delta.Reset || delta.ContentDelta != " world" || delta.ReasoningDelta != " not" {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestStreamOutboxCoalescesDeltas(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	for i, content := range []string{"a", "ab", "abc"} {
		if _, err := p.updateStream(context.Background(), streamUpdateIn{
			SessionID: "s1", TurnID: "t1", StepID: "st1", UpdateID: i + 1,
			Message: streamMessage{Role: RoleAssistant, Content: content},
		}); err != nil {
			t.Fatal(err)
		}
	}
	p.flushStreamOutbox()

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 1 {
		t.Fatalf("stream commands = %d, want 1", len(ui.commands["stream"]))
	}
	var delta streamOutDelta
	if err := json.Unmarshal(ui.commands["stream"][0], &delta); err != nil {
		t.Fatal(err)
	}
	if !delta.Reset || !delta.Coalesced || delta.ContentDelta != "abc" || delta.UpdateID != 3 {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestAppendFinalAssistantClearsStreamAndPublishesDeltas(t *testing.T) {
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

	ui.mu.Lock()
	chatStatesBefore := len(ui.states["chat"])
	ui.mu.Unlock()

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

	p.flushStreamOutbox()

	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 0 {
		t.Fatalf("stream commands after final append = %d, want 0", len(ui.commands["stream"]))
	}
	if len(ui.states["chat"]) != chatStatesBefore {
		t.Fatalf("append pushed a full chat state: %d -> %d", chatStatesBefore, len(ui.states["chat"]))
	}
	appends := ui.byMount["chat:append"]
	if len(appends) != 1 {
		t.Fatalf("chat append commands = %#v", ui.byMount)
	}
	var appendState chatAppendUI
	if err := json.Unmarshal(appends[0], &appendState); err != nil {
		t.Fatal(err)
	}
	if appendState.SessionID != "s1" || appendState.AfterSeq != 1 || appendState.ThroughSeq != 2 {
		t.Fatalf("append cursor = %+v", appendState)
	}
	if len(appendState.Messages) != 1 || appendState.Messages[0].Content != "final" {
		t.Fatalf("append messages = %+v", appendState.Messages)
	}
	traceAppends := ui.byMount["trace:append"]
	if len(traceAppends) != 1 {
		t.Fatalf("trace append commands = %#v", ui.byMount)
	}
	var traceState traceAppendUI
	if err := json.Unmarshal(traceAppends[0], &traceState); err != nil {
		t.Fatal(err)
	}
	if traceState.AfterSeq != 1 || traceState.ThroughSeq != 2 || len(traceState.Facts) != 1 ||
		traceState.Facts[0].Content != "final" {
		t.Fatalf("trace append = %+v", traceState)
	}
}

func TestAppendAdvancesCursorAcrossHostFacts(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeStepStart, Role: RoleHost,
		Meta: mustJSON(map[string]any{"turn_id": "t1", "step_id": "t1-s1", "step": 1}),
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeMessage, Role: RoleUser, Content: "hello",
		Meta: mustJSON(map[string]any{"turn_id": "t1", "step_id": "t1-s1"}),
	}}}); err != nil {
		t.Fatal(err)
	}

	ui.mu.Lock()
	defer ui.mu.Unlock()
	appends := ui.byMount["chat:append"]
	if len(appends) != 2 {
		t.Fatalf("append count = %d", len(appends))
	}
	var first, second chatAppendUI
	if err := json.Unmarshal(appends[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(appends[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.AfterSeq != 1 || first.ThroughSeq != 2 || len(first.Messages) != 0 {
		t.Fatalf("host append = %+v", first)
	}
	if second.AfterSeq != 2 || second.ThroughSeq != 3 || len(second.Messages) != 1 ||
		second.Messages[0].Content != "hello" {
		t.Fatalf("message append = %+v", second)
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
	p.flushStreamOutbox()
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.commands["stream"]) != 1 {
		t.Fatalf("stream commands = %#v", ui.commands)
	}
	var delta streamDeltaUI
	if err := json.Unmarshal(ui.commands["stream"][0], &delta); err != nil {
		t.Fatal(err)
	}
	if delta.SessionID != "s1" || !delta.Reset || delta.ContentDelta != "background" {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestAppendPublishesOnlyChangedDockFields(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{
		{Type: TypeTurnStart, Role: RoleHost, Meta: mustJSON(map[string]any{"turn_id": "t1"})},
		{Type: TypeRequestHeader, Role: RoleHost, Meta: mustJSON(map[string]any{
			"turn_id": "t1", "step_id": "t1-s1", "mode": "tool_calling", "model": "deepseek-chat",
		})},
		{Type: TypeContextSnapshot, Role: RoleHost, Meta: mustJSON(map[string]any{
			"model_context_id": "mc-1", "turn_id": "t1", "step_id": "t1-s1",
			"message_count": 2, "token_estimate": 20,
		})},
		{Type: TypeLLMUsage, Role: RoleHost, Meta: mustJSON(map[string]any{
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110},
			"model": "deepseek-chat", "context_window": 65536,
		})},
	}}); err != nil {
		t.Fatal(err)
	}
	ui.mu.Lock()
	docks := ui.byMount["dock:append"]
	ui.mu.Unlock()
	if len(docks) != 1 {
		t.Fatalf("dock appends = %#v", ui.byMount)
	}
	var dock dockAppendUI
	if err := json.Unmarshal(docks[0], &dock); err != nil {
		t.Fatal(err)
	}
	if dock.Turn == nil || dock.Turn.Status != "running" || dock.Turn.TurnID != "t1" {
		t.Fatalf("dock turn = %+v", dock.Turn)
	}
	if dock.Usage == nil || dock.Usage.Requests != 1 || dock.Usage.PromptTokens != 100 ||
		dock.Usage.LastPromptTokens != 100 || dock.Usage.ContextWindow != 65536 {
		t.Fatalf("dock usage = %+v", dock.Usage)
	}
	if dock.Context == nil || dock.Context.MessageCount != 2 || dock.Request == nil ||
		dock.Request.Model != "deepseek-chat" {
		t.Fatalf("dock context = %+v request = %+v", dock.Context, dock.Request)
	}

	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeMessage, Role: RoleTool, Content: "plain tool result",
		Meta: mustJSON(map[string]any{"turn_id": "t1", "step_id": "t1-s1"}),
	}}}); err != nil {
		t.Fatal(err)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.byMount["dock:append"]) != 1 {
		t.Fatalf("plain message appended a dock delta: %#v", ui.byMount)
	}
}

func TestStateWindowsChatAndTraceWithOlderPages(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	facts := make([]factIn, 0, 250)
	for i := 1; i <= 250; i++ {
		facts = append(facts, factIn{
			Type: TypeMessage, Role: RoleUser, Content: "m" + string(rune('a'+i%26)),
			Meta: mustJSON(map[string]any{"turn_id": "t1"}),
		})
	}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: facts}); err != nil {
		t.Fatal(err)
	}
	state, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Chat.Messages) != chatWindowMessages || !state.Chat.HasMore {
		t.Fatalf("chat window = %d more=%v", len(state.Chat.Messages), state.Chat.HasMore)
	}
	if state.Chat.FirstSeq != 52 {
		t.Fatalf("chat first_seq = %d", state.Chat.FirstSeq)
	}
	if len(state.Trace.Facts) != traceWindowFacts || !state.Trace.HasMore ||
		state.Trace.Total != 251 || state.Trace.FirstSeq != 52 {
		t.Fatalf("trace window = %d more=%v total=%d first=%d",
			len(state.Trace.Facts), state.Trace.HasMore, state.Trace.Total, state.Trace.FirstSeq)
	}

	older, err := p.messages(messagesIn{SessionID: "s1", BeforeSeq: 52, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Messages) != 50 || older.HasMore || older.FirstSeq != 2 || older.Total != 250 {
		t.Fatalf("messages page = %d more=%v first=%d total=%d",
			len(older.Messages), older.HasMore, older.FirstSeq, older.Total)
	}
	olderFacts, err := p.facts(factsIn{SessionID: "s1", BeforeSeq: 52, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(olderFacts.Facts) != 10 || !olderFacts.HasMore || olderFacts.FirstSeq != 42 ||
		olderFacts.Total != 251 {
		t.Fatalf("facts page = %d more=%v first=%d total=%d",
			len(olderFacts.Facts), olderFacts.HasMore, olderFacts.FirstSeq, olderFacts.Total)
	}
	first, err := p.facts(factsIn{SessionID: "s1", BeforeSeq: 2, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Facts) != 1 || first.HasMore || first.FirstSeq != 1 || first.Total != 251 {
		t.Fatalf("first facts page = %d more=%v first=%d", len(first.Facts), first.HasMore, first.FirstSeq)
	}
}

func TestStateWindowRespectsByteBudget(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", (3<<20)/2)
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{
		{Type: TypeMessage, Role: RoleTool, Content: big, Meta: mustJSON(map[string]any{"turn_id": "t1"})},
		{Type: TypeMessage, Role: RoleTool, Content: big, Meta: mustJSON(map[string]any{"turn_id": "t1"})},
	}}); err != nil {
		t.Fatal(err)
	}
	state, err := p.state(context.Background(), stateIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Chat.Messages) != 1 || !state.Chat.HasMore || state.Chat.FirstSeq != 3 {
		t.Fatalf("chat byte window = %d more=%v first=%d",
			len(state.Chat.Messages), state.Chat.HasMore, state.Chat.FirstSeq)
	}
	if len(state.Trace.Facts) != 1 || !state.Trace.HasMore || state.Trace.Total != 3 ||
		state.Trace.FirstSeq != 3 {
		t.Fatalf("trace byte window = %d more=%v total=%d first=%d",
			len(state.Trace.Facts), state.Trace.HasMore, state.Trace.Total, state.Trace.FirstSeq)
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

func TestChatStateExposesTurnError(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{
		SessionID: "s1",
		Facts: []factIn{
			{Type: TypeTurnStart, Role: RoleHost, Meta: mustJSON(map[string]any{"turn_id": "t1"})},
			{Type: TypeMessage, Role: RoleUser, Content: "hello", Meta: mustJSON(map[string]any{"turn_id": "t1"})},
			{Type: TypeTurnEnd, Role: RoleHost, Meta: mustJSON(map[string]any{
				"turn_id": "t1", "reason": "error", "error": "provider is not ready",
			})},
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Turns) != 1 {
		t.Fatalf("turns=%+v", state.Turns)
	}
	if state.Turns[0].Status != "error" || state.Turns[0].Error != "provider is not ready" {
		t.Fatalf("turn=%+v", state.Turns[0])
	}
}

func TestCancelledTurnNoteStaysOutOfChat(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	note := "previous turn was cancelled"
	_, err := p.append(context.Background(), appendIn{
		SessionID: "s1",
		Facts: []factIn{
			{Type: TypeTurnStart, Role: RoleHost, Meta: mustJSON(map[string]any{"turn_id": "t1"})},
			{Type: TypeMessage, Role: RoleUser, Content: "hello", Meta: mustJSON(map[string]any{"turn_id": "t1"})},
			{Type: TypeTurnEnd, Role: RoleHost, Meta: mustJSON(map[string]any{"turn_id": "t1", "reason": "cancelled"})},
			{Type: TypeMessage, Role: RoleUser, Content: note, Meta: mustJSON(map[string]any{
				"turn_id": "t1", "kind": "turn_cancelled",
			})},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range state.Messages {
		if message.Content == note {
			t.Fatalf("cancelled note leaked into chat state: %+v", message)
		}
	}
	if len(state.Turns) != 1 || state.Turns[0].Status != "cancelled" {
		t.Fatalf("turns=%+v", state.Turns)
	}

	ui.mu.Lock()
	defer ui.mu.Unlock()
	appends := ui.byMount["chat:append"]
	if len(appends) != 1 {
		t.Fatalf("chat append commands = %d", len(appends))
	}
	var appendState chatAppendUI
	if err := json.Unmarshal(appends[0], &appendState); err != nil {
		t.Fatal(err)
	}
	for _, message := range appendState.Messages {
		if message.Content == note {
			t.Fatalf("cancelled note leaked into chat append: %+v", message)
		}
	}
	if appendState.Turn == nil || appendState.Turn.Status != "cancelled" {
		t.Fatalf("turn append = %+v", appendState.Turn)
	}
}

func TestChatStateAggregatesUsageContextAndRequest(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{
		SessionID: "s1", Workspace: t.TempDir(),
		Model: "deepseek-chat", ContextWindow: 65536,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{
		SessionID: "s1",
		Facts: []factIn{
			{
				Type: TypeTurnStart, Role: RoleHost,
				Meta: mustJSON(map[string]any{"turn_id": "t1", "plugin": "agent"}),
			},
			{
				Type: TypeStepStart, Role: RoleHost,
				Meta: mustJSON(map[string]any{"turn_id": "t1", "step_id": "t1-s1", "step": 1}),
			},
			{
				Type: TypeMessage, Role: RoleUser, Content: "hello",
				Meta: mustJSON(map[string]any{"turn_id": "t1", "step_id": "t1-s1"}),
			},
			{
				Type: TypeMessage, Role: RoleTool, Content: "tool out",
				Meta: mustJSON(map[string]any{
					"turn_id": "t1", "step_id": "t1-s1",
					"tool_name": "file_read", "is_error": true, "truncated": true,
				}),
			},
			{
				Type: TypeMessage, Role: RoleAssistant, Content: "done",
				Meta: mustJSON(map[string]any{"turn_id": "t1", "step_id": "t1-s1"}),
			},
			{
				Type: TypeTurnEnd, Role: RoleHost,
				Meta: mustJSON(map[string]any{"turn_id": "t1", "reason": "completed"}),
			},
			{
				Type: TypeRequestHeader, Role: RoleHost,
				Meta: mustJSON(map[string]any{
					"turn_id": "t1", "step_id": "t1-s1", "mode": "tool_calling",
					"scheme": "tool_calling", "model": "deepseek-chat", "context_window": 65536,
				}),
			},
			{
				Type: TypeContextSnapshot, Role: RoleHost,
				Meta: mustJSON(map[string]any{
					"model_context_id": "mc-1", "turn_id": "t1", "step_id": "t1-s1",
					"message_count": 3, "token_estimate": 123,
					"model": "deepseek-chat", "context_window": 65536,
					"warnings": []string{"compacted"},
				}),
			},
			{
				Type: TypeLLMUsage, Role: RoleHost,
				Meta: mustJSON(map[string]any{
					"usage":          map[string]int{"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110},
					"model":          "deepseek-chat",
					"context_window": 65536,
				}),
			},
			{
				Type: TypeLLMUsage, Role: RoleHost,
				Meta: mustJSON(map[string]any{
					"usage":          map[string]int{"prompt_tokens": 150, "completion_tokens": 5, "total_tokens": 155},
					"model":          "deepseek-chat",
					"context_window": 65536,
				}),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	state, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Model != "deepseek-chat" || state.ContextWindow != 65536 {
		t.Fatalf("model=%q window=%d", state.Model, state.ContextWindow)
	}
	if state.Usage.PromptTokens != 250 || state.Usage.CompletionTokens != 15 ||
		state.Usage.TotalTokens != 265 || state.Usage.Requests != 2 ||
		state.Usage.LastPromptTokens != 150 {
		t.Fatalf("usage=%+v", state.Usage)
	}
	if state.Context == nil || state.Context.MessageCount != 3 ||
		state.Context.TokenEstimate != 123 || state.Context.ModelContextID != "mc-1" {
		t.Fatalf("context=%+v", state.Context)
	}
	if len(state.Context.Warnings) != 1 || state.Context.Warnings[0] != "compacted" {
		t.Fatalf("warnings=%+v", state.Context.Warnings)
	}
	if state.Request == nil || state.Request.TurnID != "t1" ||
		state.Request.Scheme != "tool_calling" || state.Request.Model != "deepseek-chat" {
		t.Fatalf("request=%+v", state.Request)
	}
	if state.Context.ContextWindow != 65536 {
		t.Fatalf("context window=%d", state.Context.ContextWindow)
	}
	if len(state.Turns) != 1 {
		t.Fatalf("turns=%+v", state.Turns)
	}
	turn := state.Turns[0]
	if turn.TurnID != "t1" || turn.Status != "completed" ||
		turn.Messages != 3 || turn.Tools != 1 ||
		turn.StartedAt == 0 || turn.EndedAt == 0 {
		t.Fatalf("turn=%+v", turn)
	}
	var toolMessage *chatMessageUI
	for i := range state.Messages {
		if state.Messages[i].Role == RoleTool {
			toolMessage = &state.Messages[i]
		}
	}
	if toolMessage == nil || toolMessage.ToolName != "file_read" || !toolMessage.IsError || !toolMessage.Truncated {
		t.Fatalf("tool message=%+v", toolMessage)
	}

	dock, err := p.dockState(context.Background(), stateIn{})
	if err != nil {
		t.Fatal(err)
	}
	if dock.SessionID != "s1" || dock.Model != "deepseek-chat" || dock.ContextWindow != 65536 {
		t.Fatalf("dock=%+v", dock)
	}
	if dock.Usage.Requests != 2 || dock.Usage.LastPromptTokens != 150 {
		t.Fatalf("dock usage=%+v", dock.Usage)
	}
	if dock.Context == nil || dock.Context.MessageCount != 3 || dock.Request == nil {
		t.Fatalf("dock context=%+v request=%+v", dock.Context, dock.Request)
	}
}
