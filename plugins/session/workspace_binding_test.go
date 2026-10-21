package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBindsSessionToWorkspaceAndFiltersHistory(t *testing.T) {
	firstPath := t.TempDir()
	secondPath := t.TempDir()
	p := newPlugin(t.TempDir())
	t.Cleanup(p.close)
	first, _, err := p.workspaces.ensure(firstPath, "one", true)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := p.workspaces.ensure(secondPath, "two", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.create(context.Background(), createIn{
		SessionID: "s1", WorkspaceID: first.WorkspaceID, Workspace: firstPath,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.create(context.Background(), createIn{
		SessionID: "s2", WorkspaceID: second.WorkspaceID,
	}); err != nil {
		t.Fatal(err)
	}
	list, err := p.list(first.WorkspaceID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].SessionID != "s1" || list.Sessions[0].WorkspaceID != first.WorkspaceID {
		t.Fatalf("list = %+v", list)
	}
	info, err := p.info(infoIn{SessionID: "s2"})
	if err != nil || info.WorkspaceID != second.WorkspaceID || !sameWorkspacePath(info.Workspace, secondPath) {
		t.Fatalf("info = %+v, err=%v", info, err)
	}
}

func TestWorkspaceRegistryPersistsSelection(t *testing.T) {
	dir := t.TempDir()
	p := newPlugin(dir)
	first, _, err := p.workspaces.ensure(t.TempDir(), "first", true)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.workspaces.ensure(t.TempDir(), "second", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.selectWorkspace(first.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	p.close()

	reloaded := newPlugin(dir)
	t.Cleanup(reloaded.close)
	current, ok := reloaded.selectedWorkspace()
	if !ok || current.WorkspaceID != first.WorkspaceID {
		t.Fatalf("current = %+v, ok=%v", current, ok)
	}
	records, selected := reloaded.workspaces.list()
	if selected != first.WorkspaceID || len(records) != 2 {
		t.Fatalf("records = %+v, selected=%q", records, selected)
	}
}

func TestWorkspacePathValidationAndDeduplication(t *testing.T) {
	dir := t.TempDir()
	p := newPlugin(dir)
	t.Cleanup(p.close)
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	viaLink, _, err := p.workspaces.ensure(link, "", false)
	if err != nil {
		t.Fatal(err)
	}
	viaTarget, created, err := p.workspaces.ensure(target, "", false)
	if err != nil || created || viaTarget.WorkspaceID != viaLink.WorkspaceID {
		t.Fatalf("target = %+v, created=%v, link=%+v, err=%v", viaTarget, created, viaLink, err)
	}
	if _, _, err := p.workspaces.ensure("relative", "", false); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestLegacyWorkspaceRegistryMigratesIntoSessionData(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "session", "sessions")
	legacyDir := filepath.Join(root, "workspace")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir()
	state := workspaceRegistryState{
		Version:    workspaceRegistryVersion,
		SelectedID: "ws-legacy",
		Workspaces: []workspaceRecord{{
			WorkspaceID: "ws-legacy", Name: "legacy", Path: path,
		}},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(legacyDir, "workspaces.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	p := newPlugin(sessionDir)
	t.Cleanup(p.close)
	current, ok := p.selectedWorkspace()
	if !ok || current.WorkspaceID != "ws-legacy" {
		t.Fatalf("current = %+v, ok=%v", current, ok)
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "workspaces.json")); err != nil {
		t.Fatalf("migrated registry missing: %v", err)
	}
}
