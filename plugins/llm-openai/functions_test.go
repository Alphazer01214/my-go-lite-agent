package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestReadSSEDispatchesReasoningAndContentDeltas(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"choices":[{"delta":{"reasoning_content":"先分析"}}]}`,
		``,
		`: keep-alive`,
		`data: {"choices":[{"delta":{"reasoning":"再判断"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"你"}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":1}}`,
		``,
		`data: {"choices":[{"delta":{"content":"好"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var got []string
	if _, err := readSSE(strings.NewReader(stream), func(channel, delta string) error {
		got = append(got, channel+":"+delta)
		return nil
	}); err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	want := []string{"thinking:先分析", "thinking:再判断", "content:你", "content:好"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
}

func TestReadSSEStopsOnErrorChunk(t *testing.T) {
	stream := `data: {"error":{"message":"bad key"}}` + "\n"
	_, err := readSSE(strings.NewReader(stream), func(string, string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("err = %v, want upstream error", err)
	}
}

func TestWireMessagesOmitsReasoningFromUpstreamContent(t *testing.T) {
	messages := wireMessages([]pluginsdk.ChatMessage{{
		Role:      pluginsdk.RoleAssistant,
		Text:      "答案",
		Reasoning: "内部推理",
	}})
	if len(messages) != 1 || messages[0].Content != "答案" {
		t.Fatalf("wire messages = %+v, want content 答案", messages)
	}
}

func TestBuildChatRequestIncludesTools(t *testing.T) {
	request := buildChatRequest("model-a", []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleUser, Text: "列出文件"},
	}, []pluginsdk.Tool{{
		Name:        "file_glob",
		Description: "List files.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}})

	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if request.Tools == nil || len(request.Tools) != 1 || request.Tools[0].Function.Name != "file_glob" {
		t.Fatalf("request tools = %+v", request.Tools)
	}
	if !strings.Contains(string(raw), `"tools"`) || !strings.Contains(string(raw), "file_glob") {
		t.Fatalf("request omitted tools: %s", raw)
	}
}

func TestWireMessagesPreservesToolCallsAndResults(t *testing.T) {
	messages := wireMessages([]pluginsdk.ChatMessage{
		{
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
			Role:       pluginsdk.RoleTool,
			ToolCallID: "call-1",
			Text:       "main.go",
		},
	})

	if len(messages) != 2 {
		t.Fatalf("messages = %+v", messages)
	}
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Function.Arguments != `{"pattern":"*.go"}` {
		t.Fatalf("assistant tool calls = %+v", messages[0])
	}
	if messages[1].Role != pluginsdk.RoleTool || messages[1].ToolCallID != "call-1" || messages[1].Content != "main.go" {
		t.Fatalf("tool result = %+v", messages[1])
	}
}

func TestReadSSEAccumulatesToolCallDeltas(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"file_glob","arguments":"{\"pattern\":"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"*.go\"}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	toolCalls, err := readSSE(strings.NewReader(stream), func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("tool calls = %+v", toolCalls)
	}
	call := toolCalls[0]
	if call.ID != "call-1" || call.FunctionCall.Name != "file_glob" {
		t.Fatalf("tool call = %+v", call)
	}
	var args map[string]string
	if err := json.Unmarshal(call.FunctionCall.Arguments, &args); err != nil || args["pattern"] != "*.go" {
		t.Fatalf("arguments = %s err = %v", call.FunctionCall.Arguments, err)
	}
}
