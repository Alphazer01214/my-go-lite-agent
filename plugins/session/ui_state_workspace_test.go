package main

import (
	"context"
	"encoding/json"
	"testing"
)

// A registry workspace switch must not blank the pane on screen. The selected
// open session joins the pushed panes even when it belongs to another
// workspace, and SelectedID stays inside the pushed set.
func TestPublishPanesKeepsSelectedAcrossWorkspaceSwitch(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	ui := newFakeUIPublisher()
	p.setUIPublisher(ui)

	pathA, pathB := t.TempDir(), t.TempDir()
	wsA, _, err := p.workspaces.ensure(pathA, "a", true)
	if err != nil {
		t.Fatal(err)
	}
	wsB, _, err := p.workspaces.ensure(pathB, "b", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.create(context.Background(), createIn{
		SessionID: "s1", WorkspaceID: wsA.WorkspaceID, Workspace: pathA,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}

	// The user picks workspace B in the picker while s1 stays on screen.
	if err := p.selectWorkspace(wsB.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	p.publishPanes(context.Background())

	ui.mu.Lock()
	defer ui.mu.Unlock()
	raw := ui.states["chat"]
	if len(raw) == 0 {
		t.Fatal("no chat state published")
	}
	var panes chatPanesStateUI
	if err := json.Unmarshal(raw[len(raw)-1], &panes); err != nil {
		t.Fatal(err)
	}
	if panes.SelectedID != "s1" {
		t.Fatalf("selected = %q, want s1", panes.SelectedID)
	}
	found := false
	for _, pane := range panes.Panes {
		if pane.SessionID == "s1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("selected pane missing from panes: %+v", panes.Panes)
	}
}

// A state read must not move or clear the Runtime selection. After the registry
// workspace drifts away from the session on screen, state() still returns that
// session so a page reload restores the view instead of an empty page.
func TestStateReadKeepsSelectionWhenWorkspaceMisaligned(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)

	pathA, pathB := t.TempDir(), t.TempDir()
	wsA, _, err := p.workspaces.ensure(pathA, "a", true)
	if err != nil {
		t.Fatal(err)
	}
	wsB, _, err := p.workspaces.ensure(pathB, "b", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.create(context.Background(), createIn{
		SessionID: "s1", WorkspaceID: wsA.WorkspaceID, Workspace: pathA,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := p.selectWorkspace(wsB.WorkspaceID); err != nil {
		t.Fatal(err)
	}

	out, err := p.state(context.Background(), stateIn{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Chat == nil || out.Chat.SessionID != "s1" {
		t.Fatalf("chat = %+v, want s1", out.Chat)
	}
	if len(out.Chats) != 1 || out.Chats[0].SessionID != "s1" {
		t.Fatalf("chats = %+v, want [s1]", out.Chats)
	}
	if got := p.selectedID(); got != "s1" {
		t.Fatalf("selection = %q after state read, want s1", got)
	}
	if out.History.SelectedID != "s1" {
		t.Fatalf("history selected = %q, want s1", out.History.SelectedID)
	}
}

// When the registry workspace has its own remembered selection, a state read
// must still resolve to the session on screen. A reload must not flip the view
// to the other workspace's session, and must not steal the selection.
func TestStateReadKeepsScreenSessionOverWorkspaceMemory(t *testing.T) {
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)

	pathA, pathB := t.TempDir(), t.TempDir()
	wsA, _, err := p.workspaces.ensure(pathA, "a", true)
	if err != nil {
		t.Fatal(err)
	}
	wsB, _, err := p.workspaces.ensure(pathB, "b", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []createIn{
		{SessionID: "s1", WorkspaceID: wsA.WorkspaceID, Workspace: pathA},
		{SessionID: "s2", WorkspaceID: wsB.WorkspaceID, Workspace: pathB},
	} {
		if _, err := p.create(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	// Visit s2 (remembered under B), then go back to s1 on screen.
	for _, id := range []string{"s2", "s1"} {
		if _, err := p.selectSession(context.Background(), selectIn{SessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.selectWorkspace(wsB.WorkspaceID); err != nil {
		t.Fatal(err)
	}

	out, err := p.state(context.Background(), stateIn{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Chat == nil || out.Chat.SessionID != "s1" {
		t.Fatalf("chat = %+v, want s1 on screen", out.Chat)
	}
	if got := p.selectedID(); got != "s1" {
		t.Fatalf("selection = %q after state read, want s1", got)
	}
}
