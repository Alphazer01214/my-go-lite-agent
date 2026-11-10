package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func testToolCatalog() toolCatalog {
	return toolCatalog{
		Generation: 7,
		Tools: []pluginsdk.Tool{{
			Name:        "file_read",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
		Names: []string{"file_read"},
		Hash:  "catalog-hash",
	}
}

func TestBuildContextViewTruncatesMessages(t *testing.T) {
	long := strings.Repeat("x", maxContextMessageBytes+128)
	messages := []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleSystem, Text: "system"},
		{Role: pluginsdk.RoleUser, Text: long, Seq: 2},
		{Role: pluginsdk.RoleAssistant, Text: "hi", Reasoning: "thinking", Seq: 3},
	}
	tools := testToolCatalog()
	snapshot := buildSnapshotMeta("s-1", "model-a", nil, messages, tools)
	view := buildContextView("live", "s-1", "t-1", "model-a", messages, tools, snapshot)

	if view.Source != "live" || view.SessionID != "s-1" || view.TurnID != "t-1" || view.Model != "model-a" {
		t.Fatalf("view = %+v", view)
	}
	if view.Usage.PromptTokens != 0 || view.Usage.TotalTokens != 0 {
		t.Fatalf("usage should be empty without a recorded fact: %+v", view.Usage)
	}
	if view.Usage.MessageCount != 3 || view.Usage.ToolCount != 1 || view.Usage.ToolCatalogGeneration != 7 {
		t.Fatalf("usage = %+v", view.Usage)
	}
	if len(view.Messages) != 3 || len(view.Tools) != 1 {
		t.Fatalf("view counts = %+v", view)
	}
	if !view.Messages[1].Truncated || len(view.Messages[1].Text) > maxContextMessageBytes {
		t.Fatalf("long message not truncated: %d", len(view.Messages[1].Text))
	}
	if !view.Truncated {
		t.Fatal("view should be marked truncated")
	}
	if view.Messages[2].Reasoning != "thinking" {
		t.Fatalf("reasoning = %q", view.Messages[2].Reasoning)
	}
	if view.Snapshot == nil || view.Snapshot.ToolCatalogGeneration != 7 {
		t.Fatalf("snapshot = %+v", view.Snapshot)
	}
}

func TestBuildContextViewTotalBudget(t *testing.T) {
	messages := make([]pluginsdk.ChatMessage, 0, 80)
	for i := 0; i < 80; i++ {
		messages = append(messages, pluginsdk.ChatMessage{
			Role: pluginsdk.RoleUser,
			Text: strings.Repeat("y", maxContextMessageBytes),
		})
	}
	view := buildContextView("derived", "s-1", "", "m", messages, testToolCatalog(), snapshotMeta{})
	if !view.Truncated {
		t.Fatal("view should be truncated")
	}
	total := 0
	for _, message := range view.Messages {
		total += len(message.Text)
	}
	if total > maxContextTotalBytes {
		t.Fatalf("total = %d, want <= %d", total, maxContextTotalBytes)
	}
	if last := view.Messages[len(view.Messages)-1]; !last.Truncated {
		t.Fatal("last message should be truncated by the total budget")
	}
}

func TestRememberContextEvictsOldSessions(t *testing.T) {
	p := &plugin{contexts: make(map[string]contextCacheEntry)}
	tools := testToolCatalog()
	for i := 0; i < maxCachedContexts+2; i++ {
		sessionID := fmt.Sprintf("s-%d", i)
		p.rememberContext(sessionID, "t-1", "m", nil, tools, snapshotMeta{SessionID: sessionID})
	}
	p.contextMu.Lock()
	count := len(p.contexts)
	last := p.lastContext
	_, hasFirst := p.contexts["s-0"]
	p.contextMu.Unlock()
	if count != maxCachedContexts {
		t.Fatalf("cache count = %d, want %d", count, maxCachedContexts)
	}
	if last != "s-9" {
		t.Fatalf("last context = %q", last)
	}
	if hasFirst {
		t.Fatal("oldest context was not evicted")
	}
}

func TestContextViewForReturnsCachedView(t *testing.T) {
	p := &plugin{contexts: make(map[string]contextCacheEntry)}
	tools := testToolCatalog()
	p.rememberContext("s-1", "t-1", "m", []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hello"}}, tools, snapshotMeta{})
	view, err := p.contextViewFor(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("contextViewFor: %v", err)
	}
	if view.SessionID != "s-1" || view.Source != "live" || len(view.Messages) != 1 {
		t.Fatalf("view = %+v", view)
	}
}

func TestContextViewForEmpty(t *testing.T) {
	p := &plugin{contexts: make(map[string]contextCacheEntry)}
	view, err := p.contextViewFor(context.Background(), "")
	if err != nil {
		t.Fatalf("contextViewFor: %v", err)
	}
	if view.Source != "none" || view.Messages == nil || view.Tools == nil {
		t.Fatalf("view = %+v", view)
	}
}

func TestUsageFromFactsUsesLatestRecordedUsage(t *testing.T) {
	facts := []sessionFact{
		{Seq: 1, Type: factTypeUsage, Meta: json.RawMessage(`{"turn_id":"t-1","step":1,"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`)},
		{Seq: 2, Type: factTypeUsage, Meta: json.RawMessage(`{"turn_id":"t-1","step":2,"prompt_tokens":20,"completion_tokens":3,"total_tokens":23}`)},
	}
	meta := usageFromFacts(facts)
	if meta == nil || meta.PromptTokens != 20 || meta.CompletionTokens != 3 || meta.TotalTokens != 23 {
		t.Fatalf("usage = %+v, want latest fact", meta)
	}

	view := contextView{}
	applyUsage(&view, meta)
	if view.Usage.PromptTokens != 20 || view.Usage.TotalTokens != 23 {
		t.Fatalf("view usage = %+v", view.Usage)
	}

	applyUsage(&view, usageFromFacts([]sessionFact{{Seq: 3, Type: factTypeMessage}}))
	if view.Usage.PromptTokens != 20 {
		t.Fatalf("missing usage fact must keep view usage: %+v", view.Usage)
	}
}

func TestTruncateContextText(t *testing.T) {
	value, truncated := truncateContextText("hello", 3, 10)
	if value != "hel" || !truncated {
		t.Fatalf("truncate = %q, %v", value, truncated)
	}
	value, truncated = truncateContextText("hello", 10, 10)
	if value != "hello" || truncated {
		t.Fatalf("no truncate = %q, %v", value, truncated)
	}
}
