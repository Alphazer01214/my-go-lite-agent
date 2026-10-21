package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type fakeSDK struct {
	mu                 sync.Mutex
	facts              []fact
	tools              []toolSpec
	nextSeq            int
	appendErr          error
	deriveTruncatedSeq []int
}

func (f *fakeSDK) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch capability + "." + method {
	case "session.derive":
		out := testProject(f.facts)
		out.TruncatedSeqs = append([]int(nil), f.deriveTruncatedSeq...)
		return mustJSON(out), nil
	case "session.append":
		if f.appendErr != nil {
			return nil, f.appendErr
		}
		var in appendIn
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		seqs := make([]int, 0, len(in.Facts))
		for _, item := range in.Facts {
			f.nextSeq++
			seqs = append(seqs, f.nextSeq)
			f.facts = append(f.facts, fact{
				Seq: f.nextSeq, Type: item.Type, Role: item.Role, Content: item.Content,
				ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID, Meta: item.Meta,
			})
		}
		return mustJSON(appendOut{SessionID: in.SessionID, Seqs: seqs, NextSeq: f.nextSeq + 1}), nil
	case "session.query":
		var in queryIn
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		types := make(map[string]bool, len(in.Types))
		for _, item := range in.Types {
			types[item] = true
		}
		out := queryOut{Facts: make([]fact, 0, len(f.facts))}
		for _, item := range f.facts {
			if len(types) > 0 && !types[item.Type] {
				continue
			}
			out.Facts = append(out.Facts, item)
		}
		out.Total = len(out.Facts)
		return mustJSON(out), nil
	case "tools.list":
		return mustJSON(map[string]any{"tools": f.tools}), nil
	default:
		return nil, pluginsdk.ErrCode(pluginsdk.CodeMethodNotFound, "unexpected call: "+capability+"."+method)
	}
}

func (f *fakeSDK) CallInContext(ctx context.Context, _ string, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	return f.Call(ctx, capability, method, payload)
}

func (f *fakeSDK) add(role, content string, meta json.RawMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextSeq++
	f.facts = append(f.facts, fact{
		Seq: f.nextSeq, Type: factTypeMessage, Role: role, Content: content, Meta: meta,
	})
}

func testProject(facts []fact) deriveOut {
	out := deriveOut{Messages: []derivedMessage{}}
	var prompt, runtime, fallback *fact
	var summary summaryFactMeta
	summarySeq := 0
	for i := range facts {
		item := &facts[i]
		if item.Type == factTypeContextSummary {
			var meta summaryFactMeta
			if len(item.Meta) > 0 {
				_ = json.Unmarshal(item.Meta, &meta)
			}
			if !meta.Active {
				continue
			}
			to := meta.ShadowedToSeq
			if to == 0 {
				to = meta.CoversThroughSeq
			}
			if summarySeq == 0 || item.Seq > summarySeq {
				summarySeq = item.Seq
				summary = meta
				summary.ShadowedToSeq = to
			}
			continue
		}
		if item.Type != factTypeMessage {
			continue
		}
		meta := parseDerivedMeta(item.Meta)
		switch meta.Kind {
		case kindSystemPrompt:
			if prompt == nil || item.Seq > prompt.Seq {
				prompt = item
			}
		case kindRuntimeContext:
			if runtime == nil || item.Seq > runtime.Seq {
				runtime = item
			}
		}
	}
	shadowed := func(seq int) bool {
		if summarySeq == 0 || seq > summary.ShadowedToSeq {
			return false
		}
		return summary.ShadowedFromSeq == 0 || seq >= summary.ShadowedFromSeq
	}
	if prompt == nil {
		for i := range facts {
			item := &facts[i]
			if item.Type != factTypeMessage || item.Role != roleSystem {
				continue
			}
			meta := parseDerivedMeta(item.Meta)
			if meta.Kind == kindSystemPrompt || meta.Kind == kindRuntimeContext {
				continue
			}
			if shadowed(item.Seq) {
				continue
			}
			if fallback == nil || item.Seq > fallback.Seq {
				fallback = item
			}
		}
	}
	out.CoveredThroughSeq = summary.ShadowedToSeq
	out.SummarySeq = summarySeq
	add := func(item *fact) {
		out.Messages = append(out.Messages, derivedMessage{
			Seq: item.Seq, Role: item.Role, Content: item.Content,
			ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID, Meta: item.Meta,
		})
	}
	head := prompt
	if head == nil {
		head = fallback
	}
	if head != nil {
		add(head)
	}
	if summarySeq != 0 {
		for i := range facts {
			if facts[i].Type == factTypeContextSummary && facts[i].Seq == summarySeq {
				out.Messages = append(out.Messages, derivedMessage{
					Seq: facts[i].Seq, Role: roleSystem,
					Content: facts[i].Content, Meta: facts[i].Meta,
				})
				break
			}
		}
	}
	if runtime != nil {
		add(runtime)
	}
	for i := range facts {
		item := &facts[i]
		if item.Type != factTypeMessage || item.Role == roleSystem {
			continue
		}
		meta := parseDerivedMeta(item.Meta)
		if meta.Kind == kindSystemPrompt || meta.Kind == kindRuntimeContext {
			continue
		}
		if shadowed(item.Seq) {
			continue
		}
		add(item)
	}
	return out
}

func testPlugin(sdk *fakeSDK) *plugin {
	p := newPlugin(sdk)
	p.cfg = config{
		PromptSections: []promptSection{
			{Name: "identity", Order: -1000, Text: "ID"},
			{Name: "persona", Order: 0, Text: "Workspace {{workspace}}"},
		},
		MaxContextTokens:    0,
		CompactThreshold:    0.8,
		RetainRecentTokens:  10000,
		CompactMaxChars:     12000,
		AgentsMDMaxBytes:    65536,
		SnapshotMemoryLimit: 128,
	}
	return p
}

func countFacts(sdk *fakeSDK, typ string) int {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	count := 0
	for _, item := range sdk.facts {
		if item.Type == typ {
			count++
		}
	}
	return count
}

func TestPrepareWritesTransparentContextFacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("workspace rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	sdk := &fakeSDK{nextSeq: 1}
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)

	meta := completeMeta{
		SessionID: "s1", TurnID: "t1", StepID: "st1",
		Mode: "coding", Scheme: "tool_calling", Workspace: dir, WorkspaceID: "ws1",
		Model: "deepseek-chat", ContextWindow: 65536,
	}
	out, err := p.prepare(context.Background(), "", meta, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 3 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if out.Messages[0].Role != roleSystem || !strings.Contains(out.Messages[0].Content, "ID") {
		t.Fatalf("system = %+v", out.Messages[0])
	}
	if out.Messages[1].Role != roleUser || !strings.Contains(out.Messages[1].Content, "workspace rules") {
		t.Fatalf("runtime = %+v", out.Messages[1])
	}
	if out.Messages[2].Content != "hello" {
		t.Fatalf("user = %+v", out.Messages[2])
	}
	if out.Snapshot.MessageCount != 3 || out.Snapshot.MessagesHash == "" || out.Snapshot.ToolsHash == "" {
		t.Fatalf("snapshot = %+v", out.Snapshot)
	}
	if out.Snapshot.Model != "deepseek-chat" || out.Snapshot.ContextWindow != 65536 {
		t.Fatalf("snapshot model = %+v", out.Snapshot)
	}
	if countFacts(sdk, factTypeMessage) != 3 {
		t.Fatalf("message facts = %d", countFacts(sdk, factTypeMessage))
	}
	if countFacts(sdk, factTypeContextSnapshot) != 1 {
		t.Fatalf("snapshot facts = %d", countFacts(sdk, factTypeContextSnapshot))
	}
	snapshot, err := p.inspect(context.Background(), "", inspectIn{ModelContextID: out.Snapshot.ModelContextID})
	if err != nil || snapshot.Snapshot.ModelContextID != out.Snapshot.ModelContextID {
		t.Fatalf("inspect = %+v, %v", snapshot, err)
	}
	if !snapshot.Found || len(snapshot.Messages) != 3 || snapshot.Integrity != "ok" {
		t.Fatalf("inspect payload = %+v", snapshot)
	}
	if snapshot.Messages[0].Source != "system_prompt" ||
		snapshot.Messages[1].Source != "runtime_context" ||
		snapshot.Messages[2].Source != "session" {
		t.Fatalf("sources = %+v", snapshot.Messages)
	}
}

func TestPrepareSkipsUnchangedSystemAndRuntime(t *testing.T) {
	dir := t.TempDir()
	sdk := &fakeSDK{nextSeq: 1}
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)
	meta := completeMeta{
		SessionID: "s1", Mode: "coding", Scheme: "tool_calling",
		Workspace: dir, WorkspaceID: "ws1",
	}
	if _, err := p.prepare(context.Background(), "", meta, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := p.prepare(context.Background(), "", meta, nil); err != nil {
		t.Fatal(err)
	}
	if countFacts(sdk, factTypeMessage) != 3 {
		t.Fatalf("message facts = %d, want system+runtime+user", countFacts(sdk, factTypeMessage))
	}
	if countFacts(sdk, factTypeContextSnapshot) != 2 {
		t.Fatalf("snapshot facts = %d", countFacts(sdk, factTypeContextSnapshot))
	}
}

func TestPrepareFoldsSessionSystemInstructions(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 1}
	sdk.add(roleSystem, "session rules", nil)
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)
	out, err := p.prepare(context.Background(), "", completeMeta{
		SessionID: "s1", Workspace: t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Messages[0].Content, "Session instructions:\nsession rules") {
		t.Fatalf("system = %q", out.Messages[0].Content)
	}
}

func TestPromptInterpolationRejectsUnknownVariable(t *testing.T) {
	_, _, err := renderPrompt([]promptSection{{Name: "bad", Text: "{{unknown}}"}}, map[string]string{})
	if err == nil || pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildDoesNotWriteFacts(t *testing.T) {
	dir := t.TempDir()
	sdk := &fakeSDK{nextSeq: 1, tools: []toolSpec{{
		Name: "file_read", Description: "read", Risk: 10,
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}}}
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)
	out, err := p.build(context.Background(), "", buildIn{
		SessionID: "s1", Mode: "coding", Scheme: "tool_calling", Workspace: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 3 || len(out.Tools) != 1 || out.Tools[0].Function.Name != "file_read" {
		t.Fatalf("out = %+v", out)
	}
	if countFacts(sdk, factTypeContextSnapshot) != 0 {
		t.Fatal("build wrote a snapshot")
	}
	if countFacts(sdk, factTypeMessage) != 1 {
		t.Fatal("build wrote context facts")
	}
}

func TestCompactWritesShadowRange(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 4}
	sdk.facts = []fact{
		{Seq: 1, Type: factTypeMessage, Role: roleUser, Content: "u1"},
		{Seq: 2, Type: factTypeMessage, Role: roleAssistant, Content: "a1"},
		{Seq: 3, Type: factTypeMessage, Role: roleUser, Content: "u2"},
		{Seq: 4, Type: factTypeMessage, Role: roleAssistant, Content: "a2"},
	}
	p := testPlugin(sdk)
	out, err := p.compact(context.Background(), "", compactIn{
		SessionID: "s1", KeepRecentTokens: 1, MaxSummaryChars: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Compacted || out.SourceFromSeq != 1 || out.SourceToSeq != 2 || out.Messages != 2 {
		t.Fatalf("out = %+v", out)
	}
	if out.SummarySeq != 5 {
		t.Fatalf("summary seq = %d", out.SummarySeq)
	}
	var summary fact
	for _, item := range sdk.facts {
		if item.Type == factTypeContextSummary {
			summary = item
		}
	}
	if summary.Seq == 0 {
		t.Fatal("missing summary fact")
	}
	var meta summaryFactMeta
	if err := json.Unmarshal(summary.Meta, &meta); err != nil {
		t.Fatal(err)
	}
	if !meta.Active || meta.ShadowedFromSeq != 1 || meta.ShadowedToSeq != 2 {
		t.Fatalf("summary meta = %+v", meta)
	}
	view, err := p.derive(context.Background(), "", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 3 || view.Messages[0].Role != roleSystem || view.Messages[1].Content != "u2" {
		t.Fatalf("view = %+v", view)
	}
}

func TestRuntimeContextCleared(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 2}
	sdk.facts = []fact{
		{Seq: 1, Type: factTypeMessage, Role: roleUser, Content: "old context",
			Meta: mustJSON(runtimeFactMeta{Source: sourceModelContext, Kind: kindRuntimeContext, Hash: "old"})},
		{Seq: 2, Type: factTypeMessage, Role: roleUser, Content: "hello"},
	}
	p := testPlugin(sdk)
	out, err := p.prepare(context.Background(), "", completeMeta{SessionID: "s1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) < 2 || out.Messages[1].Content != runtimeContextCleared {
		t.Fatalf("messages = %+v", out.Messages)
	}
}

func TestPrepareAutoCompactsWhenOverBudget(t *testing.T) {
	long := func(label string) string { return label + strings.Repeat("x", 1000) }
	sdk := &fakeSDK{nextSeq: 4}
	sdk.facts = []fact{
		{Seq: 1, Type: factTypeMessage, Role: roleUser, Content: long("u1")},
		{Seq: 2, Type: factTypeMessage, Role: roleAssistant, Content: long("a1")},
		{Seq: 3, Type: factTypeMessage, Role: roleUser, Content: long("u2")},
		{Seq: 4, Type: factTypeMessage, Role: roleAssistant, Content: long("a2")},
	}
	p := testPlugin(sdk)
	p.cfg.PromptSections = []promptSection{{Name: "id", Text: "ID"}}
	p.cfg.MaxContextTokens = 700
	p.cfg.CompactThreshold = 0.8
	p.cfg.RetainRecentTokens = 100
	p.cfg.CompactMaxChars = 500

	out, err := p.prepare(context.Background(), "", completeMeta{SessionID: "s1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(out.Warnings, "compacted") {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
	if countFacts(sdk, factTypeContextSummary) != 1 || countFacts(sdk, factTypeContextCompaction) != 1 {
		t.Fatalf("summary=%d compaction=%d",
			countFacts(sdk, factTypeContextSummary), countFacts(sdk, factTypeContextCompaction))
	}
	if len(out.Snapshot.Surface) != 4 {
		t.Fatalf("surface = %+v", out.Snapshot.Surface)
	}
	summarySeq := 0
	for _, item := range sdk.facts {
		if item.Type == factTypeContextSummary {
			summarySeq = item.Seq
		}
	}
	foundSummary := false
	for _, seq := range out.Snapshot.Surface {
		if seq == summarySeq {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Fatalf("summary seq %d missing from surface %+v", summarySeq, out.Snapshot.Surface)
	}
	if out.Snapshot.TokenEstimate > p.cfg.MaxContextTokens {
		t.Fatalf("estimate = %d", out.Snapshot.TokenEstimate)
	}
}

func TestPrepareFailsWhenBudgetCannotBeMet(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 1}
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)
	p.cfg.MaxContextTokens = 5
	_, err := p.prepare(context.Background(), "", completeMeta{SessionID: "s1"}, nil)
	if err == nil || pluginsdk.Code(err) != "context_budget_exceeded" {
		t.Fatalf("err = %v", err)
	}
}

func TestCompleteRejectsMissingMeta(t *testing.T) {
	p := testPlugin(&fakeSDK{})
	_, err := p.complete(context.Background(), &pluginsdk.Request{Payload: json.RawMessage(`{}`)})
	if err == nil || pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectBySessionReturnsSurfaceMessages(t *testing.T) {
	dir := t.TempDir()
	sdk := &fakeSDK{nextSeq: 1}
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)
	meta := completeMeta{
		SessionID: "s1", TurnID: "t1", StepID: "s1-1",
		Mode: "coding", Scheme: "tool_calling", Workspace: dir, WorkspaceID: "ws1",
		Model: "deepseek-chat", ContextWindow: 65536,
	}
	prepared, err := p.prepare(context.Background(), "", meta, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.inspect(context.Background(), "", inspectIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found || len(out.Messages) != 3 || out.Integrity != "ok" {
		t.Fatalf("inspect = %+v", out)
	}
	if out.Messages[0].Content != prepared.Messages[0].Content ||
		out.Messages[1].Content != prepared.Messages[1].Content ||
		out.Messages[2].Content != "hello" {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if out.Breakdown.SystemPrompt == 0 || out.Breakdown.RuntimeContext == 0 || out.Breakdown.Session == 0 {
		t.Fatalf("breakdown = %+v", out.Breakdown)
	}
}

func TestInspectWithoutSnapshotIsEmpty(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 1}
	sdk.add(roleUser, "hello", nil)
	p := testPlugin(sdk)
	out, err := p.inspect(context.Background(), "", inspectIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Found || len(out.Messages) != 0 {
		t.Fatalf("inspect = %+v", out)
	}
}

func TestInspectMarksTruncatedToolResults(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 4}
	toolCalls := json.RawMessage(`[{"id":"c1","type":"function","function":{"name":"file_write","arguments":"{}"}}]`)
	sdk.facts = []fact{
		{Seq: 1, Type: factTypeMessage, Role: roleUser, Content: "hello"},
		{Seq: 2, Type: factTypeMessage, Role: roleAssistant, ToolCalls: toolCalls},
		{Seq: 3, Type: factTypeMessage, Role: "tool", Content: "result", ToolCallID: "c1"},
		{Seq: 4, Type: factTypeMessage, Role: roleAssistant, Content: "done"},
	}
	stub := "[tool result omitted; 6 bytes; see seq=3]"
	hash := messagesHash([]llmMessage{
		{Role: roleUser, Content: "hello"},
		{Role: roleAssistant, ToolCalls: toolCalls},
		{Role: "tool", Content: stub, ToolCallID: "c1"},
		{Role: roleAssistant, Content: "done"},
	})
	snap := snapshot{
		ModelContextID: "mc-1", SessionID: "s1",
		Surface: []int{1, 2, 3, 4}, MessageCount: 4,
		MessagesHash: hash, TruncatedSeqs: []int{3},
	}
	sdk.facts = append(sdk.facts, fact{
		Seq: 5, Type: factTypeContextSnapshot, Role: roleHost, Meta: mustJSON(snap),
	})
	p := testPlugin(sdk)
	out, err := p.inspect(context.Background(), "", inspectIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found || out.Integrity != "ok" {
		t.Fatalf("inspect = %+v", out)
	}
	if len(out.Messages) != 4 || !out.Messages[2].Truncated || out.Messages[2].Content != stub {
		t.Fatalf("messages = %+v", out.Messages)
	}
}

func TestInspectInfersMissingTruncatedSeqs(t *testing.T) {
	sdk := &fakeSDK{nextSeq: 4, deriveTruncatedSeq: []int{3}}
	toolCalls := json.RawMessage(`[{"id":"c1","type":"function","function":{"name":"file_write","arguments":"{}"}}]`)
	sdk.facts = []fact{
		{Seq: 1, Type: factTypeMessage, Role: roleUser, Content: "hello"},
		{Seq: 2, Type: factTypeMessage, Role: roleAssistant, ToolCalls: toolCalls},
		{Seq: 3, Type: factTypeMessage, Role: "tool", Content: "result", ToolCallID: "c1"},
		{Seq: 4, Type: factTypeMessage, Role: roleAssistant, Content: "done"},
	}
	stub := "[tool result omitted; 6 bytes; see seq=3]"
	hash := messagesHash([]llmMessage{
		{Role: roleUser, Content: "hello"},
		{Role: roleAssistant, ToolCalls: toolCalls},
		{Role: "tool", Content: stub, ToolCallID: "c1"},
		{Role: roleAssistant, Content: "done"},
	})
	sdk.facts = append(sdk.facts, fact{
		Seq: 5, Type: factTypeContextSnapshot, Role: roleHost,
		Meta: mustJSON(snapshot{
			ModelContextID: "mc-old", SessionID: "s1",
			Surface: []int{1, 2, 3, 4}, MessageCount: 4, MessagesHash: hash,
		}),
	})
	p := testPlugin(sdk)
	out, err := p.inspect(context.Background(), "", inspectIn{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Integrity != "ok" || len(out.Messages) != 4 || !out.Messages[2].Truncated {
		t.Fatalf("inspect = %+v", out)
	}
	if !containsString(out.Warnings, "truncation_inferred_from_current_derive") {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
	if containsString(out.Warnings, "messages_hash_mismatch") {
		t.Fatalf("stale mismatch warning: %+v", out.Warnings)
	}
}

func TestContextWindowCapsBudget(t *testing.T) {
	long := func(label string) string { return label + strings.Repeat("x", 1000) }
	sdk := &fakeSDK{nextSeq: 4}
	sdk.facts = []fact{
		{Seq: 1, Type: factTypeMessage, Role: roleUser, Content: long("u1")},
		{Seq: 2, Type: factTypeMessage, Role: roleAssistant, Content: long("a1")},
		{Seq: 3, Type: factTypeMessage, Role: roleUser, Content: long("u2")},
		{Seq: 4, Type: factTypeMessage, Role: roleAssistant, Content: long("a2")},
	}
	p := testPlugin(sdk)
	p.cfg.PromptSections = []promptSection{{Name: "id", Text: "ID"}}
	p.cfg.MaxContextTokens = 0
	p.cfg.CompactThreshold = 0.8
	p.cfg.RetainRecentTokens = 100
	p.cfg.CompactMaxChars = 500

	// The configured cap is unlimited, so only the model window can trigger
	// compaction. This is the small-window case that previously overshot.
	out, err := p.prepare(context.Background(), "", completeMeta{
		SessionID: "s1", Model: "small", ContextWindow: 700,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(out.Warnings, "compacted") {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
	if out.Snapshot.TokenEstimate > 700 {
		t.Fatalf("estimate = %d, window = 700", out.Snapshot.TokenEstimate)
	}
	if out.Snapshot.ContextWindow != 700 || out.Snapshot.Model != "small" {
		t.Fatalf("snapshot = %+v", out.Snapshot)
	}
}
