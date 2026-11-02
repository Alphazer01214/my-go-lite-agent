package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestMessagesPreserveToolCallsAndResults(t *testing.T) {
	dir := t.TempDir()
	workspace := t.TempDir()
	p := newPluginAtDir(dir)
	defer p.close()

	meta, err := p.create(context.Background(), &createReq{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.appendFacts(context.Background(), &appendReq{
		SessionId: meta.SessionId,
		Facts: []factIn{
			{
				Type: TypeMessage,
				Role: pluginsdk.RoleAssistant,
				ToolCalls: []pluginsdk.ToolCall{{
					ID:   "call-1",
					Type: "function",
					FunctionCall: pluginsdk.FunctionCall{
						Name:      "file_glob",
						Arguments: json.RawMessage(`{"pattern":"*.go"}`),
					},
				}},
			},
			{
				Type:        TypeMessage,
				Role:        pluginsdk.RoleTool,
				ToolCallID:  "call-1",
				TextContent: "main.go",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := p.messages(context.Background(), &infoReq{SessionId: meta.SessionId})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	if len(out.Messages[0].ToolCalls) != 1 || out.Messages[0].ToolCalls[0].FunctionCall.Name != "file_glob" {
		t.Fatalf("assistant tool calls = %+v", out.Messages[0])
	}
	if out.Messages[1].Role != pluginsdk.RoleTool || out.Messages[1].ToolCallID != "call-1" || out.Messages[1].Text != "main.go" {
		t.Fatalf("tool result = %+v", out.Messages[1])
	}
}
