package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func fact(seq int, typ, role, content string, meta string) Fact {
	f := Fact{Seq: seq, Type: typ, Role: role, Content: content}
	if meta != "" {
		f.Meta = json.RawMessage(meta)
	}
	return f
}

func TestProjectMessagesBasic(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleSystem, "sys1", ""),
		fact(2, TypeMessage, RoleSystem, "sys2", ""),
		fact(3, TypeMessage, RoleUser, "hi", ""),
		fact(4, TypeMessage, RoleAssistant, "hello", ""),
	}
	out := projectMessages(facts, 8)
	if out.CoveredThroughSeq != 0 || out.SummarySeq != 0 {
		t.Fatalf("unexpected summary: %+v", out)
	}
	if len(out.Messages) != 3 {
		t.Fatalf("want 3 messages (last system only), got %d: %+v", len(out.Messages), out.Messages)
	}
	// last system stays in place (before user)
	if out.Messages[0].Role != RoleSystem || out.Messages[0].Content != "sys2" {
		t.Fatalf("want sys2 first, got %+v", out.Messages[0])
	}
	if out.Messages[1].Content != "hi" || out.Messages[2].Content != "hello" {
		t.Fatalf("order broken: %+v", out.Messages)
	}
}

func TestProjectMessagesActiveSummary(t *testing.T) {
	// sys is inside covers range → replaced by SUMMARY; tail after covers kept.
	facts := []Fact{
		fact(1, TypeMessage, RoleSystem, "sys", ""),
		fact(2, TypeMessage, RoleUser, "old", ""),
		fact(3, TypeMessage, RoleAssistant, "old-a", ""),
		fact(4, TypeContextSummary, RoleHost, "SUMMARY", `{"active":true,"covers_through_seq":3}`),
		fact(5, TypeMessage, RoleUser, "new", ""),
		fact(6, TypeMessage, RoleAssistant, "new-a", ""),
	}
	out := projectMessages(facts, 8)
	if out.CoveredThroughSeq != 3 || out.SummarySeq != 4 {
		t.Fatalf("summary meta: %+v", out)
	}
	if len(out.Messages) != 3 {
		t.Fatalf("want 3, got %d: %+v", len(out.Messages), out.Messages)
	}
	if out.Messages[0].Content != "SUMMARY" || out.Messages[0].Role != RoleSystem {
		t.Fatalf("summary at head: %+v", out.Messages[0])
	}
	if out.Messages[1].Content != "new" || out.Messages[2].Content != "new-a" {
		t.Fatalf("tail: %+v", out.Messages)
	}
}

func TestProjectMessagesSummaryAfterPostCoverSystem(t *testing.T) {
	// system written AFTER summary stays; SUMMARY inserts after it.
	facts := []Fact{
		fact(1, TypeMessage, RoleUser, "old", ""),
		fact(2, TypeContextSummary, RoleHost, "SUMMARY", `{"active":true,"covers_through_seq":1}`),
		fact(3, TypeMessage, RoleSystem, "sys-later", ""),
		fact(4, TypeMessage, RoleUser, "new", ""),
	}
	out := projectMessages(facts, 8)
	if len(out.Messages) != 3 {
		t.Fatalf("want 3, got %+v", out.Messages)
	}
	if out.Messages[0].Content != "sys-later" {
		t.Fatalf("system first: %+v", out.Messages[0])
	}
	if out.Messages[1].Content != "SUMMARY" {
		t.Fatalf("summary after system: %+v", out.Messages[1])
	}
	if out.Messages[2].Content != "new" {
		t.Fatalf("tail: %+v", out.Messages[2])
	}
}

func TestProjectMessagesInactiveSummaryIgnored(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleUser, "u", ""),
		fact(2, TypeContextSummary, RoleHost, "S", `{"active":false,"covers_through_seq":1}`),
	}
	out := projectMessages(facts, 8)
	if out.SummarySeq != 0 || len(out.Messages) != 1 {
		t.Fatalf("got %+v", out)
	}
}

func TestProjectMessagesToolStub(t *testing.T) {
	facts := []Fact{
		{
			Seq: 1, Type: TypeMessage, Role: RoleAssistant,
			ToolCalls: json.RawMessage(`[{"id":"c1"},{"id":"c2"},{"id":"c3"}]`),
		},
		{Seq: 2, Type: TypeMessage, Role: RoleTool, Content: "r1", ToolCallID: "c1"},
		{Seq: 3, Type: TypeMessage, Role: RoleTool, Content: "r2-long", ToolCallID: "c2"},
		{Seq: 4, Type: TypeMessage, Role: RoleTool, Content: "r3", ToolCallID: "c3"},
		fact(5, TypeMessage, RoleAssistant, "done", ""),
	}
	out := projectMessages(facts, 1)
	if out.TruncatedTools != 2 {
		t.Fatalf("truncated=%d", out.TruncatedTools)
	}
	if len(out.TruncatedSeqs) != 2 || out.TruncatedSeqs[0] != 2 || out.TruncatedSeqs[1] != 3 {
		t.Fatalf("truncated seqs=%+v", out.TruncatedSeqs)
	}
	if out.Messages[1].Content == "r1" || out.Messages[2].Content == "r2-long" {
		t.Fatalf("early tools not stubbed: %+v", out.Messages)
	}
	if out.Messages[3].Content != "r3" {
		t.Fatalf("last tool should stay: %+v", out.Messages[3])
	}

	all := projectMessages(facts, 0)
	if all.TruncatedTools != 3 {
		t.Fatalf("fullTool=0 want 3 stubbed, got %d", all.TruncatedTools)
	}
}

func TestProjectMessagesModelContextFacts(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleUser, "old", ""),
		fact(2, TypeMessage, RoleAssistant, "old-a", ""),
		fact(3, TypeContextSummary, RoleHost, "SUMMARY", `{"active":true,"shadowed_from_seq":1,"shadowed_to_seq":2}`),
		fact(4, TypeMessage, RoleSystem, "PROMPT", `{"source":"model-context","kind":"system_prompt","hash":"p"}`),
		fact(5, TypeMessage, RoleUser, "RUNTIME", `{"source":"model-context","kind":"runtime_context","hash":"r"}`),
		fact(6, TypeMessage, RoleUser, "new", ""),
	}
	out := projectMessages(facts, 8)
	if out.SummarySeq != 3 || out.CoveredThroughSeq != 2 {
		t.Fatalf("summary = %+v", out)
	}
	if len(out.Messages) != 4 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	wantRoles := []string{RoleSystem, RoleSystem, RoleUser, RoleUser}
	wantContent := []string{"PROMPT", "SUMMARY", "RUNTIME", "new"}
	wantSeqs := []int{4, 3, 5, 6}
	for i, message := range out.Messages {
		if message.Role != wantRoles[i] || message.Content != wantContent[i] || message.Seq != wantSeqs[i] {
			t.Fatalf("message[%d] = %+v", i, message)
		}
	}
	if len(out.Messages[0].Meta) == 0 || len(out.Messages[2].Meta) == 0 {
		t.Fatalf("derived meta missing: %+v", out.Messages)
	}
}

func TestProjectMessagesSystemPromptExemptFromShadow(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleSystem, "PROMPT", `{"source":"model-context","kind":"system_prompt","hash":"p"}`),
		fact(2, TypeMessage, RoleUser, "old", ""),
		fact(3, TypeContextSummary, RoleHost, "SUMMARY", `{"active":true,"shadowed_from_seq":1,"shadowed_to_seq":2}`),
	}
	out := projectMessages(facts, 8)
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if out.Messages[0].Content != "PROMPT" || out.Messages[1].Content != "SUMMARY" {
		t.Fatalf("order = %+v", out.Messages)
	}
}

func TestProjectMessagesKeepsLatestRuntimeContext(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleUser, "RUNTIME-1", `{"source":"model-context","kind":"runtime_context","hash":"r1"}`),
		fact(2, TypeMessage, RoleUser, "hello", ""),
		fact(3, TypeMessage, RoleUser, "RUNTIME-2", `{"source":"model-context","kind":"runtime_context","hash":"r2"}`),
	}
	out := projectMessages(facts, 8)
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if out.Messages[0].Content != "RUNTIME-2" || out.Messages[1].Content != "hello" {
		t.Fatalf("order = %+v", out.Messages)
	}
}

func TestProjectMessagesDeterministic(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleSystem, "sys", ""),
		fact(2, TypeMessage, RoleUser, "u", ""),
		fact(3, TypeContextSummary, RoleHost, "S", `{"active":true,"covers_through_seq":2}`),
	}
	a := projectMessages(facts, 8)
	b := projectMessages(facts, 8)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("not pure:\n%s\n%s", ja, jb)
	}
}

func TestProjectMessagesKeepsPairedToolResult(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleUser, "write it", ""),
		{
			Seq: 2, Type: TypeMessage, Role: RoleAssistant, Content: "",
			ToolCalls: json.RawMessage(`[{"id":"call-1","type":"function","function":{"name":"file_write","arguments":"{}"}}]`),
		},
		{Seq: 3, Type: TypeMessage, Role: RoleTool, Content: "ok", ToolCallID: "call-1"},
	}
	out := projectMessages(facts, 8)
	if len(out.Messages) != 3 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if out.Messages[1].Role != RoleAssistant || len(out.Messages[1].ToolCalls) == 0 {
		t.Fatalf("assistant = %+v", out.Messages[1])
	}
	if out.Messages[2].Role != RoleTool || out.Messages[2].ToolCallID != "call-1" {
		t.Fatalf("tool = %+v", out.Messages[2])
	}
}

func TestProjectMessagesRepairsOrphanToolResult(t *testing.T) {
	facts := []Fact{
		fact(1, TypeMessage, RoleUser, "write it", ""),
		// Older agent versions persisted the tool result but not tool_calls.
		fact(2, TypeMessage, RoleAssistant, "", ""),
		{Seq: 3, Type: TypeMessage, Role: RoleTool, Content: "file written", ToolCallID: "call-1"},
	}
	out := projectMessages(facts, 8)
	if len(out.Messages) != 3 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	recovered := out.Messages[2]
	if recovered.Role != RoleUser || recovered.ToolCallID != "" || len(recovered.ToolCalls) != 0 {
		t.Fatalf("orphan tool not repaired: %+v", recovered)
	}
	if !strings.Contains(recovered.Content, `original tool_call_id="call-1"`) ||
		!strings.Contains(recovered.Content, "file written") {
		t.Fatalf("recovered content = %q", recovered.Content)
	}
}
