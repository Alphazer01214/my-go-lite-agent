package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestCreateAppendRecover(t *testing.T) {
	dir := t.TempDir()
	workspaceDir := t.TempDir()
	canonical, err := canonicalWorkspacePath(workspaceDir)
	if err != nil {
		t.Fatalf("canonical workspace: %v", err)
	}
	p := newPluginAtDir(dir)
	defer p.close()
	ctx := context.Background()

	meta, err := p.create(ctx, &createReq{Title: "demo", Workspace: workspaceDir, Model: "model-a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if meta.Origin != "agent" {
		t.Fatalf("origin = %q, want agent", meta.Origin)
	}
	if meta.CreatedAt <= 0 {
		t.Fatalf("created_at = %d, want > 0", meta.CreatedAt)
	}
	if meta.Workspace != canonical {
		t.Fatalf("workspace binding = %q, want %q", meta.Workspace, canonical)
	}
	if meta.Model != "model-a" {
		t.Fatalf("model = %q, want model-a", meta.Model)
	}

	appended, err := p.appendFacts(ctx, &appendReq{
		SessionId: meta.SessionId,
		Facts: []factIn{
			{Type: TypeSessionStart, Role: "host"},
			{Type: "message", Role: "user", TextContent: "hi"},
			{Type: "message", Role: "assistant", TextContent: "hello", Timestamp: 1700000000123},
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if got := appended.Seqs; len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("seqs = %v, want [1 2 3]", got)
	}
	if appended.NextSeq != 4 {
		t.Fatalf("next_seq = %d, want 4", appended.NextSeq)
	}

	info, err := p.info(ctx, &infoReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.FactCnt != 3 || info.NextSeq != 4 {
		t.Fatalf("info = count %d next %d, want 3/4", info.FactCnt, info.NextSeq)
	}
	if info.Title != "demo" || info.Workspace != canonical {
		t.Fatalf("info meta = %+v", info.Meta)
	}
	if info.Model != "model-a" {
		t.Fatalf("info model = %q, want model-a", info.Model)
	}

	page, err := p.facts(ctx, &factsReq{SessionId: meta.SessionId, AfterSeq: 2, Types: []string{"message"}})
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if page.Total != 3 || page.NextSeq != 4 {
		t.Fatalf("page total/next = %d/%d, want 3/4", page.Total, page.NextSeq)
	}
	if len(page.Facts) != 1 || page.Facts[0].Seq != 3 || page.Facts[0].TextContent != "hello" {
		t.Fatalf("page facts = %+v", page.Facts)
	}
	if page.Facts[0].Timestamp != 1700000000123 {
		t.Fatalf("caller timestamp lost: %d", page.Facts[0].Timestamp)
	}

	p.close()

	// 重启后恢复计数与 meta，且新追加从下一个序号继续。
	reopened := newPluginAtDir(dir)
	defer reopened.close()
	info, err = reopened.info(ctx, &infoReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("info after reopen: %v", err)
	}
	if info.FactCnt != 3 || info.NextSeq != 4 {
		t.Fatalf("info after reopen = count %d next %d, want 3/4", info.FactCnt, info.NextSeq)
	}
	if info.Title != "demo" {
		t.Fatalf("title after reopen = %q", info.Title)
	}
	if info.Model != "model-a" {
		t.Fatalf("model after reopen = %q, want model-a", info.Model)
	}
	appended, err = reopened.appendFacts(ctx, &appendReq{
		SessionId: meta.SessionId,
		Facts:     []factIn{{Type: "message", Role: "user", TextContent: "again"}},
	})
	if err != nil {
		t.Fatalf("append after reopen: %v", err)
	}
	if appended.Seqs[0] != 4 {
		t.Fatalf("seq after reopen = %d, want 4", appended.Seqs[0])
	}
	reopened.close()

	assertFirstLineKind(t, filepath.Join(dir, meta.SessionId+".jsonl"), RecordKindMeta)
}

func TestRecoverSkipsCorruptLineAndAppliesMetaRevision(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	p := newPluginAtDir(dir)
	defer p.close()

	meta, err := p.create(ctx, &createReq{Title: "before", Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.appendFacts(ctx, &appendReq{
		SessionId: meta.SessionId,
		Facts:     []factIn{{Type: "message", Role: "user"}, {Type: "message", Role: "assistant"}},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	p.close()

	path := filepath.Join(dir, meta.SessionId+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	revision, _ := json.Marshal(metaRecord{Kind: RecordKindMeta, Meta: Meta{
		SessionId: meta.SessionId,
		Title:     "after",
		Workspace: meta.Workspace,
		Origin:    meta.Origin,
		CreatedAt: meta.CreatedAt,
	}})
	if _, err := f.WriteString(string(revision) + "\nbroken-json\n"); err != nil {
		t.Fatalf("write corrupt line: %v", err)
	}
	_ = f.Close()

	reopened := newPluginAtDir(dir)
	defer reopened.close()
	info, err := reopened.info(ctx, &infoReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Title != "after" {
		t.Fatalf("title = %q, want after", info.Title)
	}
	if info.FactCnt != 2 || info.NextSeq != 3 {
		t.Fatalf("count/next = %d/%d, want 2/3", info.FactCnt, info.NextSeq)
	}

	appended, err := reopened.appendFacts(ctx, &appendReq{
		SessionId: meta.SessionId,
		Facts:     []factIn{{Type: "message", Role: "user", TextContent: "after corrupt"}},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if appended.Seqs[0] != 3 {
		t.Fatalf("seq = %d, want 3", appended.Seqs[0])
	}

	page, err := reopened.facts(ctx, &factsReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(page.Facts) != 3 {
		t.Fatalf("facts = %d, want 3", len(page.Facts))
	}
	reopened.close()
}

func TestMissingSessionErrorGuidesCreate(t *testing.T) {
	p := newPluginAtDir(t.TempDir())
	defer p.close()

	_, err := p.info(context.Background(), &infoReq{SessionId: "s-missing"})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if got := pluginsdk.Code(err); got != codeSessionNotFound {
		t.Fatalf("code = %q, want %q", got, codeSessionNotFound)
	}
	if !strings.Contains(err.Error(), "session.create") {
		t.Fatalf("error should guide caller to session.create, got %q", err.Error())
	}
}

func TestMessagesProjectsOnlyMessageFacts(t *testing.T) {
	p := newPluginAtDir(t.TempDir())
	defer p.close()
	ctx := context.Background()

	meta, err := p.create(ctx, &createReq{Title: "demo", Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.appendFacts(ctx, &appendReq{
		SessionId: meta.SessionId,
		Facts: []factIn{
			{Type: TypeTurnStart, Role: "host", TextContent: "ignored"},
			{Type: TypeMessage, Role: "user", TextContent: "hi"},
			{Type: TypeStepStart, TextContent: "ignored"},
			{Type: TypeMessage, Role: "assistant", TextContent: "hello"},
			{Type: TypeMessage, TextContent: "no role"},
		},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	out, err := p.messages(ctx, &infoReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %+v, want 2 entries", out.Messages)
	}
	if out.Messages[0].Seq != 2 || out.Messages[0].Role != "user" || out.Messages[0].Text != "hi" {
		t.Fatalf("first message = %+v", out.Messages[0])
	}
	if out.Messages[1].Seq != 4 || out.Messages[1].Role != "assistant" || out.Messages[1].Text != "hello" {
		t.Fatalf("second message = %+v", out.Messages[1])
	}
}

func TestUpdateMetaAndDeleteSession(t *testing.T) {
	dir := t.TempDir()
	workspace := t.TempDir()
	ctx := context.Background()
	p := newPluginAtDir(dir)
	defer p.close()

	first, err := p.create(ctx, &createReq{Title: "before", Workspace: workspace, Model: "model-b"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.create(ctx, &createReq{Title: "second", Workspace: workspace}); err != nil {
		t.Fatalf("create second: %v", err)
	}

	title := "renamed"
	pinned := true
	updated, err := p.update(ctx, &updateReq{SessionId: first.SessionId, Title: &title, Pinned: &pinned})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != title || !updated.Pinned {
		t.Fatalf("updated meta = %+v", updated.Meta)
	}
	if updated.Model != "model-b" {
		t.Fatalf("model after update = %q, want model-b", updated.Model)
	}

	list, err := p.listSessions(ctx, &listReq{Workspace: first.Workspace})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Sessions) != 2 || list.Sessions[0].SessionId != first.SessionId || !list.Sessions[0].Pinned {
		t.Fatalf("pinned session should sort first: %+v", list.Sessions)
	}

	p.close()
	reopened := newPluginAtDir(dir)
	defer reopened.close()
	info, err := reopened.info(ctx, &infoReq{SessionId: first.SessionId})
	if err != nil {
		t.Fatalf("info after reopen: %v", err)
	}
	if info.Title != title || !info.Pinned {
		t.Fatalf("meta revision lost: %+v", info.Meta)
	}

	deleted, err := reopened.deleteSession(ctx, &deleteReq{SessionId: first.SessionId})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleted.Deleted || deleted.SessionId != first.SessionId {
		t.Fatalf("delete response = %+v", deleted)
	}
	if _, err := reopened.info(ctx, &infoReq{SessionId: first.SessionId}); pluginsdk.Code(err) != codeSessionNotFound {
		t.Fatalf("info after delete code = %q, want %q", pluginsdk.Code(err), codeSessionNotFound)
	}
	if _, err := os.Stat(filepath.Join(dir, first.SessionId+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("session file still exists: %v", err)
	}
}

func TestCreateWritesInitialFacts(t *testing.T) {
	dir := t.TempDir()
	workspaceDir := t.TempDir()
	p := newPluginAtDir(dir)
	defer p.close()
	ctx := context.Background()

	meta, err := p.create(ctx, &createReq{
		Title:     "bootstrap",
		Workspace: workspaceDir,
		Model:     "model-a",
		Facts: []factIn{
			{Type: "system_prompt", Role: "system", TextContent: "system text"},
			{Type: "runtime_context", Role: "user", TextContent: "runtime text"},
		},
	})
	if err != nil {
		t.Fatalf("create with facts: %v", err)
	}

	page, err := p.facts(ctx, &factsReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	if len(page.Facts) != 2 {
		t.Fatalf("fact count = %d, want 2", len(page.Facts))
	}
	if page.Facts[0].Seq != 1 || page.Facts[0].Type != "system_prompt" || page.Facts[0].TextContent != "system text" {
		t.Fatalf("first fact = %+v", page.Facts[0])
	}
	if page.Facts[1].Seq != 2 || page.Facts[1].Type != "runtime_context" || page.Facts[1].TextContent != "runtime text" {
		t.Fatalf("second fact = %+v", page.Facts[1])
	}

	info, err := p.info(ctx, &infoReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.FactCnt != 2 || info.NextSeq != 3 {
		t.Fatalf("info count/next = %d/%d, want 2/3", info.FactCnt, info.NextSeq)
	}

	if _, err := p.create(ctx, &createReq{
		Workspace: workspaceDir,
		Facts:     []factIn{{Type: ""}},
	}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("invalid initial facts code = %q, want %q", pluginsdk.Code(err), pluginsdk.CodeBadArguments)
	}
}

func assertFirstLineKind(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("log is empty")
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &head); err != nil {
		t.Fatalf("decode first line: %v", err)
	}
	if head.Kind != want {
		t.Fatalf("first line kind = %q, want %q", head.Kind, want)
	}
}
