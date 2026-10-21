package main

import (
	"context"
	"encoding/json"
	"testing"
)

// A workspace switch must leave the chat and history on the same workspace.
// The newest session of the target workspace becomes selected when the previous
// selection belongs elsewhere.
func TestPublishPanesSelectsWorkspaceSession(t *testing.T) {
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
	for _, in := range []createIn{
		{SessionID: "s1", WorkspaceID: wsA.WorkspaceID, Workspace: pathA},
		{SessionID: "s2", WorkspaceID: wsB.WorkspaceID, Workspace: pathB},
	} {
		if _, err := p.create(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.selectSession(context.Background(), selectIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}

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
	if panes.SelectedID != "s2" {
		t.Fatalf("selected = %q, want s2", panes.SelectedID)
	}
	if len(panes.Panes) != 1 || panes.Panes[0].SessionID != "s2" {
		t.Fatalf("panes = %+v, want [s2]", panes.Panes)
	}
}

// Switching to an empty workspace clears the selection. The state read must not
// resurrect a session from the previous workspace.
func TestStateReadClearsSelectionForEmptyWorkspace(t *testing.T) {
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
	if out.Chat != nil {
		t.Fatalf("chat = %+v, want nil", out.Chat)
	}
	if len(out.Chats) != 0 {
		t.Fatalf("chats = %+v, want empty", out.Chats)
	}
	if got := p.selectedID(); got != "" {
		t.Fatalf("selection = %q after workspace switch, want empty", got)
	}
	if out.History.SelectedID != "" {
		t.Fatalf("history selected = %q, want empty", out.History.SelectedID)
	}
}

// A workspace switch prefers its remembered session over the pane that was on
// screen in another workspace. State reads then resolve inside the new workspace.
func TestStateReadFollowsRememberedWorkspaceSelection(t *testing.T) {
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
	if out.Chat == nil || out.Chat.SessionID != "s2" {
		t.Fatalf("chat = %+v, want s2", out.Chat)
	}
	if got := p.selectedID(); got != "s2" {
		t.Fatalf("selection = %q after workspace switch, want s2", got)
	}
	if out.History.SelectedID != "s2" {
		t.Fatalf("history selected = %q, want s2", out.History.SelectedID)
	}
}
