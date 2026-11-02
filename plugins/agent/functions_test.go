package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestTitleFromInput(t *testing.T) {
	if got := titleFromInput("  你好\n世界  "); got != "你好 世界" {
		t.Fatalf("title = %q", got)
	}
	long := strings.Repeat("字", 35)
	got := titleFromInput(long)
	runes := []rune(got)
	if len(runes) != 31 || runes[len(runes)-1] != '…' {
		t.Fatalf("long title = %q (%d runes)", got, len(runes))
	}
	if got := titleFromInput("   \n\t "); got != "New Session" {
		t.Fatalf("empty title = %q", got)
	}
}

func TestIsCancelled(t *testing.T) {
	if !isCancelled(context.Canceled) {
		t.Fatal("context.Canceled should count as cancelled")
	}
	if !isCancelled(pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "cancelled")) {
		t.Fatal("call_cancelled should count as cancelled")
	}
	if isCancelled(errors.New("boom")) {
		t.Fatal("plain error must not count as cancelled")
	}
	if isCancelled(nil) {
		t.Fatal("nil must not count as cancelled")
	}
}

func TestNormalizeToolCalls(t *testing.T) {
	messages := []pluginsdk.ChatMessage{{
		Role: pluginsdk.RoleAssistant,
		ToolCalls: []pluginsdk.ToolCall{{
			FunctionCall: pluginsdk.FunctionCall{
				Name:      "file_glob",
				Arguments: json.RawMessage(`"{\"pattern\":\"*.go\"}"`),
			},
		}},
	}}
	if err := normalizeToolCalls(messages, "turn-1", 2); err != nil {
		t.Fatal(err)
	}
	call := messages[0].ToolCalls[0]
	if call.ID != "turn-1-tool-2-1" || call.Type != "function" {
		t.Fatalf("normalized call = %+v", call)
	}
	var args map[string]string
	if err := json.Unmarshal(call.FunctionCall.Arguments, &args); err != nil || args["pattern"] != "*.go" {
		t.Fatalf("arguments = %s err = %v", call.FunctionCall.Arguments, err)
	}
}

func TestCollectToolCalls(t *testing.T) {
	calls := collectToolCalls([]pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleAssistant, ToolCalls: []pluginsdk.ToolCall{{ID: "one"}}},
		{Role: pluginsdk.RoleAssistant, ToolCalls: []pluginsdk.ToolCall{{ID: "two"}}},
	})
	if len(calls) != 2 || calls[0].ID != "one" || calls[1].ID != "two" {
		t.Fatalf("calls = %+v", calls)
	}
}
