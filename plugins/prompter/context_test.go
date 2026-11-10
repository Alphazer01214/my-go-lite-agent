package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestProjectMessagesOrdersBaselineBeforeHistory(t *testing.T) {
	facts := []sessionFact{
		{Seq: 1, Type: factTypeSystemPrompt, Role: pluginsdk.RoleSystem, TextContent: "system"},
		{Seq: 2, Type: factTypeRuntimeContext, Role: pluginsdk.RoleUser, TextContent: "runtime"},
		{Seq: 3, Type: "turn_start", Role: "host"},
		{Seq: 4, Type: factTypeMessage, Role: pluginsdk.RoleUser, TextContent: "hello"},
		{Seq: 5, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, TextContent: "hi", Reasoning: "思考"},
	}
	messages := projectMessages(facts)
	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(messages))
	}
	wantRoles := []string{pluginsdk.RoleSystem, pluginsdk.RoleUser, pluginsdk.RoleUser, pluginsdk.RoleAssistant}
	wantSeqs := []int{1, 2, 4, 5}
	for i := range messages {
		if messages[i].Role != wantRoles[i] || messages[i].Seq != wantSeqs[i] {
			t.Fatalf("message %d = %+v, want role %s seq %d", i, messages[i], wantRoles[i], wantSeqs[i])
		}
	}
	if messages[3].Reasoning != "思考" {
		t.Fatalf("assistant reasoning = %q, want 思考", messages[3].Reasoning)
	}
}

func TestProjectMessagesUsesLatestBaselineFact(t *testing.T) {
	facts := []sessionFact{
		{Seq: 1, Type: factTypeSystemPrompt, Role: pluginsdk.RoleSystem, TextContent: "old"},
		{Seq: 2, Type: factTypeRuntimeContext, Role: pluginsdk.RoleUser, TextContent: "runtime"},
		{Seq: 3, Type: factTypeSystemPrompt, Role: pluginsdk.RoleSystem, TextContent: "new"},
	}
	messages := projectMessages(facts)
	if messages[0].Text != "new" || messages[0].Seq != 3 {
		t.Fatalf("system message = %+v, want latest fact", messages[0])
	}
	if messages[1].Text != "runtime" {
		t.Fatalf("runtime message = %+v", messages[1])
	}
}

func TestChangedBaselineFacts(t *testing.T) {
	existing := []sessionFact{
		{Seq: 1, Type: factTypeSystemPrompt, TextContent: "same"},
		{Seq: 2, Type: factTypeRuntimeContext, TextContent: "old"},
	}
	baseline := []factIn{
		newContextFact(factTypeSystemPrompt, pluginsdk.RoleSystem, "same"),
		newContextFact(factTypeRuntimeContext, pluginsdk.RoleUser, "new"),
	}
	changed := changedBaselineFacts(existing, baseline)
	if len(changed) != 1 || changed[0].Type != factTypeRuntimeContext || changed[0].TextContent != "new" {
		t.Fatalf("changed = %+v", changed)
	}
}

func TestMergeCreateFactsPrependsBaseline(t *testing.T) {
	existingFact := json.RawMessage(`{"type":"caller_fact","text_content":"x"}`)
	raw := map[string]json.RawMessage{
		"workspace": []byte(`"E:/workspace"`),
		"facts":     mustMarshal(t, []json.RawMessage{existingFact}),
	}
	merged, err := mergeCreateFacts(raw, []factIn{
		newContextFact(factTypeSystemPrompt, pluginsdk.RoleSystem, "system"),
		newContextFact(factTypeRuntimeContext, pluginsdk.RoleUser, "runtime"),
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(merged, &payload); err != nil {
		t.Fatalf("decode merged: %v", err)
	}
	var facts []factIn
	if err := json.Unmarshal(payload["facts"], &facts); err != nil {
		t.Fatalf("decode facts: %v", err)
	}
	if len(facts) != 3 || facts[0].Type != factTypeSystemPrompt || facts[1].Type != factTypeRuntimeContext || facts[2].Type != "caller_fact" {
		t.Fatalf("facts = %+v", facts)
	}
}

func TestReplaceModelPayloadPreservesMetadata(t *testing.T) {
	raw := map[string]json.RawMessage{
		"meta":     []byte(`{"session_id":"s-1"}`),
		"messages": []byte(`[{"role":"user","text":"old"}]`),
	}
	payload, err := replaceModelPayload(raw, []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleSystem, Text: "system"},
		{Role: pluginsdk.RoleUser, Text: "new"},
	}, "model-a", nil)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	var out struct {
		Meta     map[string]string       `json:"meta"`
		Model    string                  `json:"model"`
		Messages []pluginsdk.ChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Meta["session_id"] != "s-1" || out.Model != "model-a" {
		t.Fatalf("payload metadata lost: %+v", out)
	}
	if len(out.Messages) != 2 || out.Messages[1].Text != "new" {
		t.Fatalf("messages = %+v", out.Messages)
	}
}

func TestMessageHashIsStable(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleSystem, Text: strings.Repeat("a", 40)},
		{Role: pluginsdk.RoleUser, Text: "hello"},
	}
	if messageHash(messages) != messageHash(messages) {
		t.Fatal("message hash is not stable")
	}
}

func mustMarshal(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func TestProjectMessagesPreservesToolCallAndResult(t *testing.T) {
	facts := []sessionFact{
		{Seq: 1, Type: factTypeSystemPrompt, Role: pluginsdk.RoleSystem, TextContent: "system"},
		{Seq: 2, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, ToolCalls: []pluginsdk.ToolCall{{
			ID:   "call-1",
			Type: "function",
			FunctionCall: pluginsdk.FunctionCall{
				Name:      "file_glob",
				Arguments: json.RawMessage(`{"pattern":"*.go"}`),
			},
		}}},
		{Seq: 3, Type: factTypeMessage, Role: pluginsdk.RoleTool, ToolCallID: "call-1", TextContent: "main.go"},
	}
	messages := projectMessages(facts)
	if len(messages) != 3 {
		t.Fatalf("messages = %+v", messages)
	}
	if len(messages[1].ToolCalls) != 1 || messages[1].ToolCalls[0].FunctionCall.Name != "file_glob" {
		t.Fatalf("tool calls = %+v", messages[1])
	}
	if messages[2].Role != pluginsdk.RoleTool || messages[2].ToolCallID != "call-1" || messages[2].Text != "main.go" {
		t.Fatalf("tool result = %+v", messages[2])
	}
}

func TestReplaceModelPayloadOwnsModelVisibleContent(t *testing.T) {
	raw := map[string]json.RawMessage{
		"meta":     []byte(`{"session_id":"s-1"}`),
		"model":    []byte(`"caller-model"`),
		"messages": []byte(`[{"role":"user","text":"caller text"}]`),
		"tools":    []byte(`[{"name":"caller_tool"}]`),
		"stream":   []byte(`true`),
	}
	messages := []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "projected"}}
	tools := []pluginsdk.Tool{{
		Name:        "file_glob",
		Description: "List files.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}}

	payload, err := replaceModelPayload(raw, messages, "session-model", tools)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatal(err)
	}
	if string(out["model"]) != `"session-model"` {
		t.Fatalf("model = %s", out["model"])
	}
	if string(out["messages"]) != `[{"role":"user","text":"projected"}]` {
		t.Fatalf("messages = %s", out["messages"])
	}
	if !strings.Contains(string(out["tools"]), `"file_glob"`) || strings.Contains(string(out["tools"]), "caller_tool") {
		t.Fatalf("tools = %s", out["tools"])
	}
	if _, ok := out["stream"]; ok {
		t.Fatalf("caller-only model field leaked: %s", payload)
	}
	if string(out["meta"]) != `{"session_id":"s-1"}` {
		t.Fatalf("meta = %s", out["meta"])
	}
}

func TestToolCatalogHashIsStable(t *testing.T) {
	catalog := newToolCatalog(7, []pluginsdk.Tool{{
		Name:        "file_glob",
		Description: "List files.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}})
	if catalog.Generation != 7 || len(catalog.Tools) != 1 || catalog.Tools[0].Name != "file_glob" {
		t.Fatalf("catalog = %+v", catalog)
	}
	if catalog.Hash == "" || catalog.Hash != toolCatalogHash(catalog.Tools) {
		t.Fatalf("catalog hash = %q", catalog.Hash)
	}
}
