// Command llm-openai is an OpenAI-compatible LLM Plugin (DeepSeek and others).
//
// Capability: llm
//   - complete: Call Payload {"messages":[...]}; streams presentation.stream chunk evt;
//     res {"content"} or {"content":"","tool_calls":[...]}
//
// Config: env OPENAI_API_KEY / OPENAI_BASE_URL / OPENAI_MODEL override config.json
// beside the executable. Defaults target DeepSeek's OpenAI-compatible API.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const (
	defaultBaseURL = "https://api.deepseek.com/v1"
	defaultModel   = "deepseek-chat"
)

type config struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

func loadConfig() config {
	cfg := config{
		BaseURL: defaultBaseURL,
		Model:   defaultModel,
	}
	if exe, err := os.Executable(); err == nil {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "config.json"))
		if err == nil {
			_ = json.Unmarshal(raw, &cfg)
		}
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		cfg.Model = v
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return cfg
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
	Stream   bool          `json:"stream,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type streamDelta struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func toWireMessages(in []struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolCalls []struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"tool_calls"`
	ToolCallID string `json:"tool_call_id"`
}) []chatMessage {
	out := make([]chatMessage, 0, len(in))
	used := map[string]bool{}
	for _, m := range in {
		cm := chatMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			if tc.Name == "" {
				continue
			}
			var w toolCall
			w.ID = tc.ID
			if w.ID == "" || used[w.ID] {
				w.ID = fmt.Sprintf("call_%d", len(used)+1)
				for used[w.ID] {
					w.ID = w.ID + "x"
				}
			}
			used[w.ID] = true
			w.Type = "function"
			w.Function.Name = tc.Name
			w.Function.Arguments = string(tc.Arguments)
			if w.Function.Arguments == "" {
				w.Function.Arguments = "{}"
			}
			cm.ToolCalls = append(cm.ToolCalls, w)
		}
		if cm.Role == "tool" && cm.ToolCallID == "" && len(cm.ToolCalls) == 0 {
			// Tool results require a tool_call_id; skip malformed rows rather than 400 the API.
			continue
		}
		if len(cm.ToolCalls) == 0 && cm.Role == "assistant" && cm.Content == "" {
			continue
		}
		out = append(out, cm)
	}
	return out
}

func toWireTools(in []struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}) []chatTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]chatTool, 0, len(in))
	for _, t := range in {
		if t.Name == "" {
			continue
		}
		var ct chatTool
		ct.Type = "function"
		ct.Function.Name = t.Name
		ct.Function.Description = t.Description
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		ct.Function.Parameters = params
		out = append(out, ct)
	}
	return out
}

func complete(cfg config, reqID string, s *pluginsdk.Server, sessionID string, messages []chatMessage, tools []chatTool) (json.RawMessage, error) {
	if cfg.APIKey == "" {
		return nil, &protocol.FrameError{
			Code:    "missing_api_key",
			Message: "set OPENAI_API_KEY or config.json apiKey",
		}
	}
	body := chatRequest{
		Model:    cfg.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := cfg.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &protocol.FrameError{Code: "llm_http_error", Message: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &protocol.FrameError{
			Code:    "llm_http_status",
			Message: fmt.Sprintf("%s: %s", resp.Status, strings.TrimSpace(string(msg))),
		}
	}

	// Stream SSE; fall back to non-stream JSON if content-type is not event-stream.
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "event-stream") {
		var cr chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
			return nil, &protocol.FrameError{Code: "llm_bad_response", Message: err.Error()}
		}
		if cr.Error != nil {
			return nil, &protocol.FrameError{Code: "llm_api_error", Message: cr.Error.Message}
		}
		if len(cr.Choices) == 0 {
			return nil, &protocol.FrameError{Code: "llm_empty_choices", Message: "no choices in response"}
		}
		msg := cr.Choices[0].Message
		if msg.Content != "" {
			payload, _ := json.Marshal(map[string]string{"op": "chunk", "delta": msg.Content, "channel": "content"})
			_ = s.EmitTo(reqID, "presentation", "stream", payload)
		}
		return marshalOut(msg)
	}

	var content strings.Builder
	type accCall struct {
		id, name, args string
	}
	var calls []accCall
	callIdx := map[int]int{}

	// Plugin-owned Session Log: one reasoning fact per model hop.
	// Live Thinking is presentation stream only; durable log is a single append
	// after the hop (Host does not interpret channel).
	var reasonAcc strings.Builder

	_ = s.EmitTo(reqID, "presentation", "stream", json.RawMessage(`{"op":"start"}`))
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var d streamDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			continue
		}
		if len(d.Choices) == 0 {
			continue
		}
		delta := d.Choices[0].Delta
		// DeepSeek (and OpenAI-compatible reasoning models) stream thinking in reasoning_content.
		if delta.ReasoningContent != "" {
			payload, _ := json.Marshal(map[string]string{
				"op":      "chunk",
				"delta":   delta.ReasoningContent,
				"channel": "reasoning",
			})
			_ = s.EmitTo(reqID, "presentation", "stream", payload)
			reasonAcc.WriteString(delta.ReasoningContent)
		}
		if delta.Content != "" {
			content.WriteString(delta.Content)
			payload, _ := json.Marshal(map[string]string{"op": "chunk", "delta": delta.Content, "channel": "content"})
			_ = s.EmitTo(reqID, "presentation", "stream", payload)
		}
		for _, tc := range delta.ToolCalls {
			i, ok := callIdx[tc.Index]
			if !ok {
				i = len(calls)
				callIdx[tc.Index] = i
				calls = append(calls, accCall{id: tc.ID, name: tc.Function.Name})
			}
			if tc.ID != "" {
				calls[i].id = tc.ID
			}
			if tc.Function.Name != "" {
				calls[i].name = tc.Function.Name
			}
			calls[i].args += tc.Function.Arguments
		}
	}
	// Persist the whole hop's thinking as one Session Log fact.
	if reasonAcc.Len() > 0 {
		appendReasoningFact(s, sessionID, reasonAcc.String())
	}
	if err := sc.Err(); err != nil {
		return nil, &protocol.FrameError{Code: "llm_stream_error", Message: err.Error()}
	}
	_ = s.EmitTo(reqID, "presentation", "stream", json.RawMessage(`{"op":"end"}`))

	msg := chatMessage{Role: "assistant", Content: content.String()}
	for _, c := range calls {
		var w toolCall
		w.ID = c.id
		if w.ID == "" {
			w.ID = fmt.Sprintf("call-%d", len(msg.ToolCalls)+1)
		}
		w.Type = "function"
		w.Function.Name = c.name
		w.Function.Arguments = c.args
		if w.Function.Arguments == "" {
			w.Function.Arguments = "{}"
		}
		msg.ToolCalls = append(msg.ToolCalls, w)
	}
	return marshalOut(msg)
}

func marshalOut(msg chatMessage) (json.RawMessage, error) {
	type outCall struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	out := map[string]any{"content": msg.Content}
	if len(msg.ToolCalls) > 0 {
		list := make([]outCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if !json.Valid(args) {
				args = json.RawMessage(`{}`)
			}
			list = append(list, outCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
		}
		out["tool_calls"] = list
	}
	return json.Marshal(out)
}

// appendReasoningFact persists thinking into Session Log via session.append.
// Best-effort: missing session plugin or routing errors do not fail the LLM call.
func appendReasoningFact(s *pluginsdk.Server, sessionID, content string) {
	if content == "" {
		return
	}
	fact := map[string]any{
		"type":    "reasoning",
		"role":    "assistant",
		"content": content,
		// Always tag the Session (empty = default) so Web can filter foreign turns.
		"sessionId": sessionID,
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return
	}
	_, _ = s.Call("session", "append", payload)
}

func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func saveConfig(cfg config) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), raw, 0o600)
}

func handleConfigCommand(args string) (json.RawMessage, error) {
	cfg := loadConfig()
	fields := strings.Fields(args)
	if len(fields) == 0 || fields[0] == "get" {
		// Never print the raw API key.
		key := cfg.APIKey
		if key != "" {
			if len(key) > 8 {
				key = key[:4] + "…" + key[len(key)-4:]
			} else {
				key = "…"
			}
		}
		text := fmt.Sprintf("baseURL=%s\nmodel=%s\napiKey=%s", cfg.BaseURL, cfg.Model, key)
		return json.Marshal(map[string]string{"text": text})
	}
	if fields[0] != "set" {
		return nil, &protocol.FrameError{
			Code:    "bad_command",
			Message: "usage: /llm-openai config [get|set key=value ...]",
		}
	}
	changed := false
	for _, kv := range fields[1:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, &protocol.FrameError{
				Code:    "bad_assignment",
				Message: fmt.Sprintf("expected key=value, got %q", kv),
			}
		}
		switch k {
		case "apiKey":
			cfg.APIKey = v
		case "baseURL":
			cfg.BaseURL = strings.TrimRight(v, "/")
		case "model":
			cfg.Model = v
		default:
			return nil, &protocol.FrameError{
				Code:    "unknown_key",
				Message: fmt.Sprintf("unknown config key %q (apiKey|baseURL|model)", k),
			}
		}
		changed = true
	}
	if !changed {
		return nil, &protocol.FrameError{Code: "bad_command", Message: "set requires at least one key=value"}
	}
	if err := saveConfig(cfg); err != nil {
		return nil, &protocol.FrameError{Code: "save_failed", Message: err.Error()}
	}
	return json.Marshal(map[string]string{"text": "config saved to " + configPath()})
}

func main() {
	s := pluginsdk.New()
	cfg := loadConfig()
	s.Handle("llm", "complete", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
			Messages  []struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID        string          `json:"id"`
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"input_schema"`
			} `json:"tools"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		return complete(cfg, req.ID, s, in.SessionID, toWireMessages(in.Messages), toWireTools(in.Tools))
	})
	s.Handle("commands", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Command string `json:"command"`
			Args    string `json:"args"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		if in.Command != "config" {
			return nil, &protocol.FrameError{
				Code:    "unknown_command",
				Message: fmt.Sprintf("unknown command %q", in.Command),
			}
		}
		return handleConfigCommand(in.Args)
	})
	_ = s.Serve()
}
