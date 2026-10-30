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
		{Seq: 5, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, TextContent: "hi"},
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

func TestReplaceMessagesPreservesPayload(t *testing.T) {
	raw := map[string]json.RawMessage{
		"meta":     []byte(`{"session_id":"s-1"}`),
		"messages": []byte(`[{"role":"user","text":"old"}]`),
	}
	payload, err := replaceMessages(raw, []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleSystem, Text: "system"},
		{Role: pluginsdk.RoleUser, Text: "new"},
	}, "model-a")
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

func TestMessageHashAndTokenEstimateAreStable(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleSystem, Text: strings.Repeat("a", 40)},
		{Role: pluginsdk.RoleUser, Text: "hello"},
	}
	if messageHash(messages) != messageHash(messages) {
		t.Fatal("message hash is not stable")
	}
	if estimateTokens(messages) <= 0 {
		t.Fatal("token estimate must be positive")
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
