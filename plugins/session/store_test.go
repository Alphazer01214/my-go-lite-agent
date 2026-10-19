package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllRecoversInterruptedTurn(t *testing.T) {
	dir := t.TempDir()
	facts := []Fact{
		{Seq: 1, Type: TypeSessionStart, Role: RoleHost, Meta: mustJSON(Meta{SessionID: "s1"})},
		{Seq: 2, Type: TypeTurnStart, Role: RoleHost, Meta: mustJSON(map[string]any{"turn_id": "t1"})},
		{Seq: 3, Type: TypeMessage, Role: RoleUser, Content: "hello", Meta: mustJSON(map[string]any{"turn_id": "t1"})},
	}
	data := make([]byte, 0)
	for _, fact := range facts {
		line, err := json.Marshal(fact)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	p := newPlugin(dir)
	t.Cleanup(p.close)
	p.loadAll()

	got, _, err := p.readFacts("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 4 {
		t.Fatalf("recovered facts = %#v, want original facts plus terminal", got)
	}
	if got[0].Type != TypeSessionStart || got[1].Type != TypeTurnStart {
		t.Fatalf("recovery rewrote the beginning of the log: %#v", got[:2])
	}
	found := false
	for _, fact := range got {
		if fact.Type != TypeTurnEnd {
			continue
		}
		if fact.Seq != 4 {
			t.Fatalf("recovered turn_end seq = %d, want 4", fact.Seq)
		}
		var meta map[string]any
		_ = json.Unmarshal(fact.Meta, &meta)
		if meta["turn_id"] == "t1" && meta["reason"] == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("facts = %#v", got)
	}
}

type fakeHostStateSDK struct {
	state string
}

func (f fakeHostStateSDK) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if capability != "host" || method != "plugins" {
		return nil, errHandler("unexpected call")
	}
	return mustJSON(map[string]any{"plugins": []map[string]string{{"name": "agent", "state": f.state}}}), nil
}

func TestChatStateRecoversWhenTurnOwnerIsDown(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	p.sdk = fakeHostStateSDK{state: "failed"}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeTurnStart,
		Role: RoleHost,
		Meta: mustJSON(map[string]any{"turn_id": "t1", "plugin": "agent"}),
	}}}); err != nil {
		t.Fatal(err)
	}

	state, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn.Status != "error" {
		t.Fatalf("chat state = %+v", state)
	}
	facts, _, err := p.readFacts("s1")
	if err != nil {
		t.Fatal(err)
	}
	last := facts[len(facts)-1]
	if last.Type != TypeTurnEnd {
		t.Fatalf("last fact = %#v", last)
	}
	var meta map[string]any
	_ = json.Unmarshal(last.Meta, &meta)
	if meta["reason"] != "error" {
		t.Fatalf("recovery meta = %#v", meta)
	}
}

func TestChatStateKeepsActiveTurnRunning(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	p.sdk = fakeHostStateSDK{state: "active"}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeTurnStart,
		Role: RoleHost,
		Meta: mustJSON(map[string]any{"turn_id": "t1", "plugin": "agent"}),
	}}}); err != nil {
		t.Fatal(err)
	}

	state, err := p.chatState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn.Status != "running" {
		t.Fatalf("chat state = %+v", state)
	}
	facts, _, err := p.readFacts("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 || facts[len(facts)-1].Type != TypeTurnStart {
		t.Fatalf("active turn was modified: %#v", facts)
	}
}
