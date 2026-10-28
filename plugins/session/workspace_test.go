package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestWorkspaceRegistryAndSessionScoping(t *testing.T) {
	sessionDir := t.TempDir()
	wsOnePath := t.TempDir()
	wsTwoPath := t.TempDir()
	canonicalOne, err := canonicalWorkspacePath(wsOnePath)
	if err != nil {
		t.Fatalf("canonical ws one: %v", err)
	}
	canonicalTwo, err := canonicalWorkspacePath(wsTwoPath)
	if err != nil {
		t.Fatalf("canonical ws two: %v", err)
	}

	p := newPluginAtDir(sessionDir)
	defer p.close()
	ctx := context.Background()

	created, err := p.createWorkspace(ctx, &workspaceReq{Path: wsOnePath})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if !created.Created || created.Workspace.Path != canonicalOne {
		t.Fatalf("created = %+v", created)
	}
	if created.SelectedId != created.Workspace.WorkspaceId {
		t.Fatalf("selected = %q, want new workspace", created.SelectedId)
	}
	if created.Workspace.Name != filepath.Base(canonicalOne) {
		t.Fatalf("name = %q", created.Workspace.Name)
	}
	wsOneId := created.Workspace.WorkspaceId

	duplicate, err := p.createWorkspace(ctx, &workspaceReq{Path: wsOnePath, Name: "ignored"})
	if err != nil {
		t.Fatalf("duplicate workspace: %v", err)
	}
	if duplicate.Created || duplicate.Workspace.WorkspaceId != wsOneId {
		t.Fatalf("duplicate = %+v", duplicate)
	}

	renamed, err := p.updateWorkspace(ctx, &workspaceReq{WorkspaceId: wsOneId, Name: "renamed"})
	if err != nil {
		t.Fatalf("rename workspace: %v", err)
	}
	if renamed.Workspace.Name != "renamed" {
		t.Fatalf("name = %q, want renamed", renamed.Workspace.Name)
	}

	if _, err := p.createWorkspace(ctx, &workspaceReq{Path: "relative/dir"}); pluginsdk.Code(err) != codeWorkspaceInvalid {
		t.Fatalf("relative path error = %v", err)
	}

	first, err := p.create(ctx, &createReq{Title: "one", WorkspaceId: wsOneId})
	if err != nil {
		t.Fatalf("create session one: %v", err)
	}
	second, err := p.create(ctx, &createReq{Title: "two", Workspace: wsTwoPath})
	if err != nil {
		t.Fatalf("create session two: %v", err)
	}
	if second.Workspace != canonicalTwo {
		t.Fatalf("session two workspace = %q, want %q", second.Workspace, canonicalTwo)
	}
	wsTwoId := second.WorkspaceId

	defaultList, err := p.listSessions(ctx, &listReq{})
	if err != nil {
		t.Fatalf("list default: %v", err)
	}
	if defaultList.WorkspaceId != wsOneId || len(defaultList.Sessions) != 1 || defaultList.Sessions[0].SessionId != first.SessionId {
		t.Fatalf("default list = %+v", defaultList)
	}
	if defaultList.Sessions[0].FactCnt != 0 || defaultList.Sessions[0].NextSeq != 1 {
		t.Fatalf("session summary = %+v", defaultList.Sessions[0])
	}

	secondList, err := p.listSessions(ctx, &listReq{WorkspaceId: wsTwoId})
	if err != nil {
		t.Fatalf("list ws two: %v", err)
	}
	if len(secondList.Sessions) != 1 || secondList.Sessions[0].SessionId != second.SessionId {
		t.Fatalf("ws two list = %+v", secondList)
	}

	if _, err := p.selectWorkspace(ctx, &workspaceReq{WorkspaceId: wsTwoId}); err != nil {
		t.Fatalf("select workspace: %v", err)
	}
	current, err := p.currentWorkspace(ctx)
	if err != nil {
		t.Fatalf("current workspace: %v", err)
	}
	if current.Workspace == nil || current.Workspace.WorkspaceId != wsTwoId {
		t.Fatalf("current = %+v", current)
	}

	if _, err := p.deleteWorkspace(ctx, &workspaceReq{WorkspaceId: wsOneId}); pluginsdk.Code(err) != codeWorkspaceNotEmpty {
		t.Fatalf("delete non-empty error = %v", err)
	}

	wsThreePath := t.TempDir()
	third, err := p.createWorkspace(ctx, &workspaceReq{Path: wsThreePath})
	if err != nil {
		t.Fatalf("create empty workspace: %v", err)
	}
	deleted, err := p.deleteWorkspace(ctx, &workspaceReq{WorkspaceId: third.Workspace.WorkspaceId})
	if err != nil {
		t.Fatalf("delete empty workspace: %v", err)
	}
	if !deleted.Deleted || deleted.SelectedId == third.Workspace.WorkspaceId {
		t.Fatalf("deleted = %+v", deleted)
	}

	if _, err := p.selectWorkspace(ctx, &workspaceReq{WorkspaceId: wsTwoId}); err != nil {
		t.Fatalf("reselect ws two: %v", err)
	}
	defaulted, err := p.create(ctx, &createReq{Title: "default workspace"})
	if err != nil {
		t.Fatalf("create default workspace session: %v", err)
	}
	if defaulted.WorkspaceId != wsTwoId {
		t.Fatalf("default workspace = %q, want %q", defaulted.WorkspaceId, wsTwoId)
	}

	if _, err := p.create(ctx, &createReq{WorkspaceId: wsTwoId, Workspace: wsOnePath}); pluginsdk.Code(err) != codeWorkspaceMismatch {
		t.Fatalf("mismatch error = %v", err)
	}
	if _, err := p.selectWorkspace(ctx, &workspaceReq{WorkspaceId: "ws-missing"}); pluginsdk.Code(err) != codeWorkspaceNotFound {
		t.Fatalf("missing workspace error = %v", err)
	}

	p.close()

	reopened := newPluginAtDir(sessionDir)
	defer reopened.close()
	workspaces, selected := reopened.workspaces.list()
	if selected != wsTwoId {
		t.Fatalf("selected after reopen = %q, want %q", selected, wsTwoId)
	}
	foundRenamed := false
	for _, record := range workspaces {
		if record.WorkspaceId == wsOneId && record.Name == "renamed" {
			foundRenamed = true
		}
	}
	if !foundRenamed {
		t.Fatalf("rename was not persisted: %+v", workspaces)
	}
	list, err := reopened.listSessions(ctx, &listReq{})
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	if list.WorkspaceId != wsTwoId || len(list.Sessions) != 2 {
		t.Fatalf("list after reopen = %+v", list)
	}
}

func TestWorkspaceCapabilityRegistered(t *testing.T) {
	p := newPluginAtDir(t.TempDir())
	defer p.close()
	p.registerHandlers()

	registered := make(map[string]bool)
	for _, entry := range p.sdk.ListRegistered() {
		registered[entry.Capability+"."+entry.Method] = true
	}
	for _, key := range []string{
		"session.list",
		"workspace.list",
		"workspace.create",
		"workspace.update",
		"workspace.select",
		"workspace.delete",
		"workspace.current",
	} {
		if !registered[key] {
			t.Fatalf("handler %s is not registered", key)
		}
	}
}

func TestLegacySessionBindsWorkspaceByPath(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceDir := t.TempDir()
	canonical, err := canonicalWorkspacePath(workspaceDir)
	if err != nil {
		t.Fatalf("canonical workspace: %v", err)
	}

	id := "s-legacy-1"
	metaLine, err := json.Marshal(metaRecord{Kind: RecordKindMeta, Meta: Meta{
		SessionId: id,
		Title:     "legacy",
		Workspace: canonical,
		Origin:    "agent",
		CreatedAt: 123,
	}})
	if err != nil {
		t.Fatalf("encode meta: %v", err)
	}
	path := filepath.Join(sessionDir, id+".jsonl")
	if err := os.WriteFile(path, append(metaLine, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy log: %v", err)
	}

	p := newPluginAtDir(sessionDir)
	defer p.close()
	ctx := context.Background()

	info, err := p.info(ctx, &infoReq{SessionId: id})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.WorkspaceId == "" || info.Workspace != canonical {
		t.Fatalf("legacy binding = %+v", info.Meta)
	}

	list, err := p.listSessions(ctx, &listReq{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].SessionId != id {
		t.Fatalf("legacy list = %+v", list)
	}
}
