package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type toolLoopHost struct {
	reader  *os.File
	writer  *os.File
	mu      sync.Mutex
	calls   []string
	appends []json.RawMessage
	llmReq  []json.RawMessage
	llm     int
}

func (h *toolLoopHost) record(call string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, call)
}

func (h *toolLoopHost) count(call string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, item := range h.calls {
		if item == call {
			count++
		}
	}
	return count
}

func (h *toolLoopHost) llmRequest(index int) json.RawMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	if index < 0 || index >= len(h.llmReq) {
		return nil
	}
	return append(json.RawMessage(nil), h.llmReq[index]...)
}

func (h *toolLoopHost) hasAppend(fragment string) bool {
	return h.appendCount(fragment) > 0
}

func (h *toolLoopHost) appendCount(fragment string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, payload := range h.appends {
		if strings.Contains(string(payload), fragment) {
			count++
		}
	}
	return count
}

func (h *toolLoopHost) run(t *testing.T) {
	t.Helper()
	for {
		frame, err := protocol.ReadFrame(h.reader)
		if err != nil {
			return
		}
		if frame.Type != protocol.FrameRequest {
			continue
		}
		call := frame.Capability + "." + frame.Method
		h.record(call)
		var payload json.RawMessage
		switch call {
		case "session.create":
			payload = json.RawMessage(`{"session_id":"s-test","workspace":"/tmp","origin":"agent","model":"model-a","created_at":1}`)
		case "session.info":
			payload = json.RawMessage(`{"session_id":"s-test","workspace":"/tmp","model":"model-a","fact_count":0,"next_seq":1}`)
		case "session.append":
			h.mu.Lock()
			h.appends = append(h.appends, append(json.RawMessage(nil), frame.Payload...))
			h.mu.Unlock()
			payload = json.RawMessage(`{"session_id":"s-test","seqs":[1],"next_seq":2}`)
		case "session.messages":
			payload = json.RawMessage(`{"messages":[]}`)
		case "tool.call":
			payload = json.RawMessage(`{"content":"listing","is_error":false,"truncated":true,"meta":{"count":2}}`)
		case "llm.stream":
			h.mu.Lock()
			h.llmReq = append(h.llmReq, append(json.RawMessage(nil), frame.Payload...))
			h.llm++
			llmCall := h.llm
			h.mu.Unlock()
			if llmCall == 1 {
				payload = json.RawMessage(`{"model":"model-a","usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12},"messages":[{"role":"assistant","text":"","tool_calls":[{"id":"call-1","type":"function","function_call":{"name":"file_glob","arguments":"{\"pattern\":\"*.go\"}"}}]}]}`)
			} else {
				payload = json.RawMessage(`{"model":"model-a","usage":{"prompt_tokens":20,"completion_tokens":3,"total_tokens":23},"messages":[{"role":"assistant","text":"done"}]}`)
			}
		default:
			t.Errorf("unexpected call %s", call)
			payload = json.RawMessage(`{}`)
		}
		response := &protocol.Frame{
			Version:    protocol.FrameVersion,
			ID:         frame.ID,
			Type:       protocol.FrameResponse,
			Capability: frame.Capability,
			Method:     frame.Method,
			Payload:    payload,
		}
		if err := protocol.WriteFrame(h.writer, response); err != nil {
			return
		}
	}
}

func newToolLoopPlugin(t *testing.T, maxSteps int) (*plugin, *toolLoopHost) {
	t.Helper()
	pluginStdoutReader, pluginStdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	pluginStdinReader, pluginStdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin, oldStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = pluginStdinReader, pluginStdoutWriter
	sdk := pluginsdk.NewPlugin("agent")
	os.Stdin, os.Stdout = oldStdin, oldStdout

	host := &toolLoopHost{reader: pluginStdoutReader, writer: pluginStdinWriter}
	go host.run(t)
	go func() { _ = sdk.Listen() }()
	t.Cleanup(func() {
		_ = pluginStdinReader.Close()
		_ = pluginStdinWriter.Close()
		_ = pluginStdoutReader.Close()
		_ = pluginStdoutWriter.Close()
	})

	p := &plugin{
		sdk:   sdk,
		cfg:   config{MaxSteps: maxSteps},
		turns: make(map[string]*activeTurn),
	}
	return p, host
}

func TestAgentLoopExecutesToolCallsAndContinues(t *testing.T) {
	p, host := newToolLoopPlugin(t, 3)
	resp, err := p.turn(context.Background(), &pluginsdk.Request{ID: "call-1"}, &fromWebUI{
		Workspace: "/tmp",
		UserInput: "call a tool",
		Model:     "model-a",
	})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if got := host.count("tool.call"); got != 1 {
		t.Fatalf("tool.call count = %d, want 1; calls=%v", got, host.calls)
	}
	if got := host.count("llm.stream"); got != 2 {
		t.Fatalf("llm.stream count = %d, want 2; calls=%v", got, host.calls)
	}
	if !host.hasAppend(`"role":"tool"`) || !host.hasAppend(`"tool_call_id":"call-1"`) || !host.hasAppend(`"text_content":"listing"`) || !host.hasAppend(`"meta":{"count":2,"is_error":false,"truncated":true}`) {
		t.Fatalf("tool result fact not appended; appends=%v", host.appends)
	}
	if len(resp.Messages) == 0 || !strings.Contains(resp.Messages[len(resp.Messages)-1].Text, "done") {
		t.Fatalf("response messages = %+v, want final answer", resp.Messages)
	}
	if got := host.appendCount(`"type":"usage"`); got != 2 {
		t.Fatalf("usage fact count = %d, want 2; appends=%v", got, host.appends)
	}
	if !host.hasAppend(`"type":"usage","role":"host","meta":{"turn_id"`) || !host.hasAppend(`"total_tokens":23`) {
		t.Fatalf("usage meta not recorded; appends=%v", host.appends)
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal(host.llmRequest(0), &request); err != nil {
		t.Fatal(err)
	}
	if _, ok := request["messages"]; ok {
		t.Fatalf("agent leaked model-visible messages: %s", request["messages"])
	}
	if _, ok := request["tools"]; ok {
		t.Fatalf("agent leaked tool definitions: %s", request["tools"])
	}
	if !strings.Contains(string(request["meta"]), `"session_id":"s-test"`) {
		t.Fatalf("llm request missing session meta: %s", request["meta"])
	}
}

func TestAgentLoopStopsAtMaxSteps(t *testing.T) {
	p, host := newToolLoopPlugin(t, 1)
	_, err := p.turn(context.Background(), &pluginsdk.Request{ID: "call-1"}, &fromWebUI{
		Workspace: "/tmp",
		UserInput: "call a tool",
		Model:     "model-a",
	})
	if err == nil || pluginsdk.Code(err) != "max_steps_exceeded" {
		t.Fatalf("turn error = %v, want max_steps_exceeded", err)
	}
	if got := host.count("llm.stream"); got != 1 {
		t.Fatalf("llm.stream count = %d, want 1", got)
	}
	if got := host.count("tool.call"); got != 0 {
		t.Fatalf("tool.call count = %d, want 0 after max steps", got)
	}
}
