package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdatePersistsNameAndPinned(t *testing.T) {
	dir := t.TempDir()
	p := newPlugin(dir)
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	name, pinned := "发布计划", true
	if _, err := p.updateSession(context.Background(), updateIn{
		SessionID: "s1", Name: &name, Pinned: &pinned,
	}); err != nil {
		t.Fatal(err)
	}

	info, err := p.info(infoIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != name || !info.Pinned {
		t.Fatalf("info = %+v", info)
	}

	// The pin and name are written as session_meta facts and survive reload.
	restarted := newPlugin(dir)
	t.Cleanup(restarted.close)
	restarted.loadAll()
	info, err = restarted.info(infoIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != name || !info.Pinned {
		t.Fatalf("reloaded info = %+v", info)
	}
}

func TestListOrdersPinnedSessionsFirst(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	workspace := t.TempDir()
	for _, id := range []string{"old", "new"} {
		if _, err := p.create(context.Background(), createIn{SessionID: id, Workspace: workspace}); err != nil {
			t.Fatal(err)
		}
	}
	pinned := true
	if _, err := p.updateSession(context.Background(), updateIn{SessionID: "old", Pinned: &pinned}); err != nil {
		t.Fatal(err)
	}
	out, err := p.list("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 2 || out.Sessions[0].SessionID != "old" || !out.Sessions[0].Pinned {
		t.Fatalf("sessions = %+v", out.Sessions)
	}
}

func TestDeleteSessionRemovesLogAndReselects(t *testing.T) {
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

	out, err := p.deleteSession(context.Background(), deleteIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Deleted || out.Selected != "s2" {
		t.Fatalf("delete out = %+v", out)
	}
	if _, err := p.get("s1"); err == nil {
		t.Fatal("deleted session is still registered")
	}
	path := filepath.Join(p.dir, "s1.jsonl")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("session file still exists: %v", err)
	}
	if ids := p.openIDs(); len(ids) != 1 || ids[0] != "s2" {
		t.Fatalf("open ids = %v", ids)
	}
}

func TestDeleteSessionRefusesOpenTurn(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	if _, err := p.create(context.Background(), createIn{SessionID: "s1", Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.append(context.Background(), appendIn{SessionID: "s1", Facts: []factIn{{
		Type: TypeTurnStart,
		Role: RoleHost,
		Meta: mustJSON(map[string]any{"turn_id": "t1"}),
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.deleteSession(context.Background(), deleteIn{SessionID: "s1"}); err == nil {
		t.Fatal("delete succeeded while a turn was open")
	}
	if _, err := p.get("s1"); err != nil {
		t.Fatalf("session removed after refused delete: %v", err)
	}
}
