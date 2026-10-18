package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type fakeModelContextSDK struct {
	appends []appendIn
}

func (f *fakeModelContextSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	switch capability + "." + method {
	case "session.derive":
		return mustJSON(map[string]any{"messages": []chatMessage{
			{Role: "system", Content: "session rules"},
			{Role: "user", Content: "hello"},
		}}), nil
	case "tools.list":
		return mustJSON(toolsListOut{Tools: []toolSpec{
			{Name: "file_read", Description: "read", Risk: "low", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
			{Name: "shell_execute", Description: "shell", Risk: "high", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		}}), nil
	case "session.append":
		var in appendIn
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		f.appends = append(f.appends, in)
		return mustJSON(appendOut{Seqs: []int{len(f.appends)}}), nil
	case "session.query":
		return mustJSON(queryOut{Facts: []fact{
			{Seq: 1, Type: "message", Role: "user", Content: "one"},
			{Seq: 2, Type: "message", Role: "assistant", Content: "two"},
			{Seq: 3, Type: "message", Role: "user", Content: "three"},
		}}), nil
	default:
		return nil, pluginsdk.ErrCode(pluginsdk.CodeMethodNotFound, "unexpected call")
	}
}

func (f *fakeModelContextSDK) CallInContext(ctx context.Context, _, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	return f.Call(ctx, capability, method, payload)
}

func TestBuildAddsSystemFiltersToolsAndPersistsSnapshot(t *testing.T) {
	sdk := &fakeModelContextSDK{}
	p := newPlugin(sdk)
	p.cfg = config{SystemPrompt: "base prompt", MaxContextChars: 10000}
	out, err := p.build(context.Background(), "ctx-1", buildIn{
		SessionID: "s1", TurnID: "t1", StepID: "st1", Scheme: "tool_calling",
		DeniedTools: []string{"shell_execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelContextID == "" || len(out.Messages) != 2 || out.Messages[0].Role != "system" {
		t.Fatalf("out = %+v", out)
	}
	if out.Messages[0].Content != "base prompt\n\nSession instructions:\nsession rules" {
		t.Fatalf("system = %q", out.Messages[0].Content)
	}
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "file_read" {
		t.Fatalf("tools = %+v", out.Tools)
	}
	if len(sdk.appends) != 1 || sdk.appends[0].Facts[0].Type != "context_snapshot" {
		t.Fatalf("appends = %+v", sdk.appends)
	}
	snap, err := p.inspect(inspectIn{ModelContextID: out.ModelContextID})
	if err != nil || snap.Hash == "" || snap.TokenEstimate <= 0 {
		t.Fatalf("snapshot = %+v, %v", snap, err)
	}
}

func TestCompactAppendsActiveSummary(t *testing.T) {
	sdk := &fakeModelContextSDK{}
	p := newPlugin(sdk)
	keep := 1
	out, err := p.compact(context.Background(), "", compactIn{SessionID: "s1", KeepLast: &keep})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Compacted || out.SourceFrom != 1 || out.SourceTo != 2 || out.Messages != 2 {
		t.Fatalf("out = %+v", out)
	}
	if len(sdk.appends) != 1 || len(sdk.appends[0].Facts) != 2 {
		t.Fatalf("appends = %+v", sdk.appends)
	}
	if sdk.appends[0].Facts[0].Type != "context_summary" {
		t.Fatalf("facts = %+v", sdk.appends[0].Facts)
	}
}

func TestTrimMessagesKeepsSystem(t *testing.T) {
	messages := []chatMessage{
		{Role: "system", Content: "prompt"},
		{Role: "user", Content: "1234567890"},
		{Role: "assistant", Content: "abcdefghij"},
	}
	got, trimmed := trimMessages(messages, 25)
	if trimmed != 1 || len(got) != 2 || got[0].Role != "system" {
		t.Fatalf("got = %+v trimmed=%d", got, trimmed)
	}
}
