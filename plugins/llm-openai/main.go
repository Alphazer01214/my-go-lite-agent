// Command llm-openai is an OpenAI-compatible LLM Plugin (DeepSeek and others).
//
// Capability: llm
//   - complete: Call Payload {"messages":[...]}; streams presentation.stream chunk evt;
//     res {"content"} or {"content":"","tool_calls":[...]} plus OpenAI-format
//     usage {"prompt_tokens","completion_tokens","total_tokens"} when the
//     provider reports it (Host records llm_usage / Context Usage).
//
// Config: env OPENAI_API_KEY / OPENAI_BASE_URL / OPENAI_MODEL override config.json
// beside the executable. Defaults target DeepSeek's OpenAI-compatible API.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const (
	defaultBaseURL = "https://api.deepseek.com/v1"
	defaultModel   = "deepseek-chat"
	// defaultContextWindow is used when config/env leave contextWindow unset.
	defaultContextWindow = 65536
	// streamDeadline caps total generation time; header stall is cut earlier below.
	streamDeadline = 10 * time.Minute
)

// sharedHTTPClient reuses TLS connections and fails header stalls quickly.
// A per-request Client.Timeout would abort long streams mid-body.
var sharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

type config struct {
	BaseURL       string `json:"baseURL"`
	APIKey        string `json:"apiKey"`
	Model         string `json:"model"`
	ContextWindow int    `json:"contextWindow,omitempty"`
}

func loadConfig() config {
	cfg := config{
		BaseURL:       defaultBaseURL,
		Model:         defaultModel,
		ContextWindow: defaultContextWindow,
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
	if v := os.Getenv("OPENAI_CONTEXT_WINDOW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ContextWindow = n
		}
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = defaultContextWindow
	}
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

// streamOptions asks OpenAI-compatible APIs to emit a final usage chunk
// (empty choices + usage object). Providers that ignore it still stream fine.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	Tools         []chatTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
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
	// Usage arrives on the final chunk when stream_options.include_usage is set.
	Usage *Usage `json:"usage"`
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

// streamPayload builds a presentation.stream body. sessionId is required so
// Web session-view can attribute live chunks (Host forwards it on the SSE event).
func streamPayload(sessionID, op, channel, delta string) json.RawMessage {
	m := map[string]string{"op": op}
	if channel != "" {
		m["channel"] = channel
	}
	if delta != "" {
		m["delta"] = delta
	}
	if sessionID != "" {
		m["sessionId"] = sessionID
	}
	b, _ := json.Marshal(m)
	return b
}

// noteUsage pushes provider token usage into Context Manager (star call).
// Best-effort: missing context-manager must not fail the model hop.
func noteUsage(s *pluginsdk.Server, sessionID string, usage *Usage) {
	if s == nil || usage == nil || sessionID == "" {
		return
	}
	u := usageOrNil(*usage)
	if u == nil {
		return
	}
	body, err := json.Marshal(map[string]any{"sessionId": sessionID, "usage": u})
	if err != nil {
		return
	}
	_, _ = s.Call("context", "noteUsage", body)
}

func complete(cfg config, reqID string, s *pluginsdk.Server, sessionID string, messages []chatMessage, tools []chatTool) (json.RawMessage, error) {
	if cfg.APIKey == "" {
		return nil, &protocol.FrameError{
			Code:    "missing_api_key",
			Message: "set OPENAI_API_KEY or config.json apiKey",
		}
	}
	body := chatRequest{
		Model:         cfg.Model,
		Messages:      messages,
		Tools:         tools,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := cfg.BaseURL + "/chat/completions"
	// ResponseHeaderTimeout fails dead endpoints fast; body/stream may run long
	// (no Client.Timeout — that would cancel mid-generation).
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := sharedHTTPClient.Do(httpReq)
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
			_ = s.EmitTo(reqID, "presentation", "stream", streamPayload(sessionID, "chunk", "content", msg.Content))
		}
		usage := cr.Usage
		noteUsage(s, sessionID, usageOrNil(usage))
		return marshalOut(msg, usageOrNil(usage))
	}

	var content strings.Builder
	type accCall struct {
		id, name, args string
	}
	var calls []accCall
	callIdx := map[int]int{}
	var usage *Usage

	// Plugin-owned Session Log: one reasoning fact per model hop.
	// Live Thinking is presentation stream only; durable log is a single append
	// after the hop (Host does not interpret channel).
	var reasonAcc strings.Builder

	_ = s.EmitTo(reqID, "presentation", "stream", streamPayload(sessionID, "start", "", ""))
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
		// Final usage chunk has empty choices (stream_options.include_usage).
		if d.Usage != nil {
			usage = d.Usage
		}
		if len(d.Choices) == 0 {
			continue
		}
		delta := d.Choices[0].Delta
		// DeepSeek (and OpenAI-compatible reasoning models) stream thinking in reasoning_content.
		if delta.ReasoningContent != "" {
			_ = s.EmitTo(reqID, "presentation", "stream", streamPayload(sessionID, "chunk", "reasoning", delta.ReasoningContent))
			reasonAcc.WriteString(delta.ReasoningContent)
		}
		if delta.Content != "" {
			content.WriteString(delta.Content)
			_ = s.EmitTo(reqID, "presentation", "stream", streamPayload(sessionID, "chunk", "content", delta.Content))
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
	// Persist the whole hop's thinking as one Session Log fact (off the return path).
	if reasonAcc.Len() > 0 {
		text := reasonAcc.String()
		go appendReasoningFact(s, sessionID, text)
	}
	if err := sc.Err(); err != nil {
		return nil, &protocol.FrameError{Code: "llm_stream_error", Message: err.Error()}
	}
	_ = s.EmitTo(reqID, "presentation", "stream", streamPayload(sessionID, "end", "", ""))

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
	noteUsage(s, sessionID, usage)
	return marshalOut(msg, usage)
}

// usageOrNil returns nil when the provider reported no tokens (omit usage key).
func usageOrNil(u Usage) *Usage {
	if u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0 {
		return nil
	}
	return &u
}

func marshalOut(msg chatMessage, usage *Usage) (json.RawMessage, error) {
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
	// OpenAI-format usage: Host appends llm_usage fact and Context Manager prefers it.
	if usage != nil && (usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0) {
		out["usage"] = usage
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

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) > 8 {
		return key[:4] + "…" + key[len(key)-4:]
	}
	return "…"
}

func configText(cfg config) string {
	return fmt.Sprintf("baseURL=%s\nmodel=%s\napiKey=%s\ncontextWindow=%d",
		cfg.BaseURL, cfg.Model, maskKey(cfg.APIKey), cfg.ContextWindow)
}

func applyConfigKey(cfg *config, k, v string) error {
	switch k {
	case "apiKey":
		cfg.APIKey = v
	case "baseURL":
		cfg.BaseURL = strings.TrimRight(v, "/")
	case "model":
		cfg.Model = v
	case "contextWindow":
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return &protocol.FrameError{Code: "bad_assignment", Message: "contextWindow must be a positive integer"}
		}
		cfg.ContextWindow = n
	default:
		return &protocol.FrameError{
			Code:    "unknown_key",
			Message: fmt.Sprintf("unknown config key %q (apiKey|baseURL|model|contextWindow)", k),
		}
	}
	return nil
}

func handleConfigCommand(args string) (json.RawMessage, error) {
	cfg := loadConfig()
	fields := strings.Fields(args)
	if len(fields) == 0 || fields[0] == "get" {
		return json.Marshal(map[string]string{"text": configText(cfg)})
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
		if err := applyConfigKey(&cfg, k, v); err != nil {
			return nil, err
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

func handleConfigCap(method string, payload json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "reload":
		cfg := loadConfig()
		return json.Marshal(map[string]any{"ok": true, "contextWindow": cfg.ContextWindow, "model": cfg.Model})
	case "get":
		cfg := loadConfig()
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "baseURL", "value": cfg.BaseURL, "type": "string"},
				{"name": "apiKey", "value": maskKey(cfg.APIKey), "secret": true, "type": "string"},
				{"name": "model", "value": cfg.Model, "type": "string"},
				{"name": "contextWindow", "value": cfg.ContextWindow, "type": "integer"},
			},
		})
	case "schema":
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "baseURL", "type": "string", "description": "OpenAI-compatible API base URL"},
				{"name": "apiKey", "type": "string", "secret": true, "description": "API key"},
				{"name": "model", "type": "string", "description": "Model id"},
				{"name": "contextWindow", "type": "integer", "default": defaultContextWindow, "description": "Max model context tokens"},
			},
		})
	case "set":
		var in map[string]any
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		cfg := loadConfig()
		for k, raw := range in {
			v := fmt.Sprint(raw)
			if err := applyConfigKey(&cfg, k, v); err != nil {
				return nil, err
			}
		}
		if err := saveConfig(cfg); err != nil {
			return nil, &protocol.FrameError{Code: "save_failed", Message: err.Error()}
		}
		return json.Marshal(map[string]any{"ok": true})
	default:
		return nil, &protocol.FrameError{Code: "unknown_method", Message: "config." + method}
	}
}

// callStat is one model-call observation (per process / per Session).
// "模型总时长" on the Host status bar reads it via llm.stats.
type callStat struct {
	Requests int   `json:"requests"`
	TotalMs  int64 `json:"totalMs"`
	LastMs   int64 `json:"lastMs"`
	Tokens   int   `json:"tokens"`
}

var (
	statMu     sync.Mutex
	statAll    callStat
	statBySess = map[string]*callStat{}
)

// noteCallStat folds one completed llm.complete into the process counters.
func noteCallStat(sessionID string, ms int64, tokens int) {
	statMu.Lock()
	defer statMu.Unlock()
	statAll.Requests++
	statAll.TotalMs += ms
	statAll.LastMs = ms
	statAll.Tokens += tokens
	if sessionID == "" {
		return
	}
	st := statBySess[sessionID]
	if st == nil {
		st = &callStat{}
		statBySess[sessionID] = st
	}
	st.Requests++
	st.TotalMs += ms
	st.LastMs = ms
	st.Tokens += tokens
}

// totalTokensOf digs provider usage out of a complete response payload.
func totalTokensOf(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var out struct {
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Usage == nil {
		return 0
	}
	return out.Usage.TotalTokens
}

func main() {
	s := pluginsdk.New()
	// Hot-reload: never capture startup cfg in the complete closure (config set / reload
	// must take effect on the next model hop without restarting the process).
	s.Handle("llm", "complete", func(req *pluginsdk.Request) (json.RawMessage, error) {
		cfg := loadConfig()
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
		start := time.Now()
		out, err := complete(cfg, req.ID, s, in.SessionID, toWireMessages(in.Messages), toWireTools(in.Tools))
		noteCallStat(in.SessionID, time.Since(start).Milliseconds(), totalTokensOf(out))
		return out, err
	})
	s.Handle("llm", "info", func(req *pluginsdk.Request) (json.RawMessage, error) {
		cfg := loadConfig()
		return json.Marshal(map[string]any{
			"contextWindow": cfg.ContextWindow,
			"model":         cfg.Model,
			"provider":      "openai-compatible",
		})
	})
	// llm.stats: model-call counters for the Host status bar (model name, total
	// model time, request count). Process-local observation, not a truth source.
	s.Handle("llm", "stats", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		cfg := loadConfig()
		statMu.Lock()
		all := statAll
		var sess *callStat
		if st := statBySess[in.SessionID]; st != nil {
			cp := *st
			sess = &cp
		}
		statMu.Unlock()
		avg := int64(0)
		if all.Requests > 0 {
			avg = all.TotalMs / int64(all.Requests)
		}
		res := map[string]any{
			"provider": "openai-compatible",
			"model":    cfg.Model,
			"requests": all.Requests,
			"totalMs":  all.TotalMs,
			"avgMs":    avg,
			"lastMs":   all.LastMs,
			"tokens":   all.Tokens,
		}
		if sess != nil {
			res["session"] = sess
		}
		return json.Marshal(res)
	})
	s.Handle("config", "get", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("get", req.Payload)
	})
	s.Handle("config", "set", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("set", req.Payload)
	})
	s.Handle("config", "schema", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("schema", req.Payload)
	})
	s.Handle("config", "reload", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("reload", req.Payload)
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
