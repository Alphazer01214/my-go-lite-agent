package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	calls, used, err := readSSE(strings.NewReader(stream), func(channel, delta string) error {
		got = append(got, channel+":"+delta)
		return nil
	})
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	want := []string{"thinking:先分析", "thinking:再判断", "content:你", "content:好"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
	if len(calls) != 0 {
		t.Fatalf("tool calls = %+v, want none", calls)
	}
	if used.PromptTokens != 1 {
		t.Fatalf("usage = %+v, want prompt_tokens 1", used)
	}
}

func TestReadSSEStopsOnErrorChunk(t *testing.T) {
	stream := `data: {"error":{"message":"bad key"}}` + "\n"
	_, _, err := readSSE(strings.NewReader(stream), func(string, string) error { return nil })
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
	}}, 0)

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
	if !strings.Contains(string(raw), `"stream_options":{"include_usage":true}`) {
		t.Fatalf("request omitted include_usage: %s", raw)
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

func TestBuildChatRequestIncludesMaxTokens(t *testing.T) {
	messages := []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hi"}}
	raw, err := json.Marshal(buildChatRequest("model-a", messages, nil, 123))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"max_tokens":123`) {
		t.Fatalf("max_tokens missing: %s", raw)
	}
	raw, err = json.Marshal(buildChatRequest("model-a", messages, nil, 0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "max_tokens") {
		t.Fatalf("zero max_tokens must be omitted: %s", raw)
	}
}

func TestConfigTimeoutsHaveDefaults(t *testing.T) {
	cfg := config{}
	if got := cfg.requestTimeout(); got != 15*time.Minute {
		t.Fatalf("request timeout = %s, want 15m", got)
	}
	if got := cfg.streamIdleTimeout(); got != 2*time.Minute {
		t.Fatalf("idle timeout = %s, want 2m", got)
	}
	if got := cfg.maxResponseBytes(); got != 4*1024*1024 {
		t.Fatalf("max response bytes = %d, want 4 MiB", got)
	}
}

func TestCappedReaderStopsOversizedResponse(t *testing.T) {
	_, err := io.ReadAll(&cappedReader{reader: strings.NewReader("abcde"), remaining: 3})
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("err = %v, want errResponseTooLarge", err)
	}
	data, err := io.ReadAll(&cappedReader{reader: strings.NewReader("abcde"), remaining: 5})
	if err != nil || string(data) != "abcde" {
		t.Fatalf("exact fit data = %q err = %v", data, err)
	}
}

func TestStreamUpstreamStatusHidesDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream secret detail", http.StatusBadGateway)
	}))
	defer server.Close()

	p := &plugin{
		cfg: config{
			DefaultModel: "model-a",
			Models:       []ModelConfig{{ID: "model-a", BaseURL: server.URL}},
		},
		client: server.Client(),
	}
	_, err := p.stream(context.Background(), &pluginsdk.Request{ID: "test"}, &streamReq{
		Model:    "model-a",
		Messages: []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hi"}},
	})
	if pluginsdk.Code(err) != codeLLMUpstreamError {
		t.Fatalf("code = %q err = %v, want %s", pluginsdk.Code(err), err, codeLLMUpstreamError)
	}
	if strings.Contains(err.Error(), "secret detail") {
		t.Fatalf("upstream detail leaked to caller: %v", err)
	}
}

func TestStreamReturnsProviderUsage(t *testing.T) {
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		stream := strings.Join([]string{
			`data: {"choices":[{"delta":{}}]}`,
			``,
			`data: {"choices":[],"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
		_, _ = io.WriteString(w, stream)
	}))
	defer server.Close()

	p := &plugin{
		cfg: config{
			DefaultModel: "model-a",
			Models:       []ModelConfig{{ID: "model-a", BaseURL: server.URL}},
		},
		client: server.Client(),
	}
	resp, err := p.stream(context.Background(), &pluginsdk.Request{ID: "test"}, &streamReq{
		Model:    "model-a",
		Messages: []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("usage missing from stream response")
	}
	if resp.Usage.PromptTokens != 120 || resp.Usage.CompletionTokens != 30 || resp.Usage.TotalTokens != 150 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if !strings.Contains(string(requestBody), `"stream_options":{"include_usage":true}`) {
		t.Fatalf("request omitted include_usage: %s", requestBody)
	}
}

func TestStreamIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	p := &plugin{
		cfg: config{
			DefaultModel:        "model-a",
			Models:              []ModelConfig{{ID: "model-a", BaseURL: server.URL}},
			RequestTimeoutMS:    60000,
			StreamIdleTimeoutMS: 50,
		},
		client: server.Client(),
	}
	_, err := p.stream(context.Background(), &pluginsdk.Request{ID: "test"}, &streamReq{
		Model:    "model-a",
		Messages: []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hi"}},
	})
	if pluginsdk.Code(err) != codeLLMTimeout {
		t.Fatalf("code = %q err = %v, want %s", pluginsdk.Code(err), err, codeLLMTimeout)
	}
}

func TestStreamRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	p := &plugin{
		cfg: config{
			DefaultModel:        "model-a",
			Models:              []ModelConfig{{ID: "model-a", BaseURL: server.URL}},
			RequestTimeoutMS:    50,
			StreamIdleTimeoutMS: 600000,
		},
		client: server.Client(),
	}
	_, err := p.stream(context.Background(), &pluginsdk.Request{ID: "test"}, &streamReq{
		Model:    "model-a",
		Messages: []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hi"}},
	})
	if pluginsdk.Code(err) != codeLLMTimeout {
		t.Fatalf("code = %q err = %v, want %s", pluginsdk.Code(err), err, codeLLMTimeout)
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

	toolCalls, _, err := readSSE(strings.NewReader(stream), func(string, string) error { return nil })
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
