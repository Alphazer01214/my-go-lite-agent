package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	codeLLMUpstreamError = "llm_upstream_error"
	codeLLMTimeout       = "llm_timeout"
	codeLLMOutputLimit   = "llm_output_limit_exceeded"
)

var errResponseTooLarge = errors.New("llm response exceeds max_response_bytes")

type idleReader struct {
	reader io.Reader
	touch  func()
}

func (r idleReader) Read(p []byte) (int, error) {
	r.touch()
	return r.reader.Read(p)
}

type cappedReader struct {
	reader    io.Reader
	remaining int64
}

func (r *cappedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, errResponseTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func (p *plugin) stream(ctx context.Context, req *pluginsdk.Request, in *streamReq) (*streamResp, error) {
	if len(in.Messages) == 0 {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "messages is required")
	}
	cfg := p.snapshotConfig()
	model, err := cfg.model(in.Model)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(buildChatRequest(model.ID, in.Messages, in.Tools, model.MaxTokens))
	if err != nil {
		return nil, err
	}

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	totalCtx, totalCancel := context.WithTimeout(streamCtx, cfg.requestTimeout())
	defer totalCancel()

	var idleExpired atomic.Bool
	idleTimeout := cfg.streamIdleTimeout()
	idleTimer := time.AfterFunc(idleTimeout, func() {
		idleExpired.Store(true)
		streamCancel()
	})
	defer idleTimer.Stop()
	touch := func() { idleTimer.Reset(idleTimeout) }

	url := strings.TrimRight(model.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(totalCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if model.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+model.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, p.streamFailure(ctx, totalCtx, &idleExpired, url, err, idleTimeout)
	}
	defer resp.Body.Close()
	touch()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if trimmed := strings.TrimSpace(string(detail)); trimmed != "" {
			fmt.Fprintf(os.Stderr, "llm-openai: upstream %d: %s\n", resp.StatusCode, trimmed)
		}
		return nil, pluginsdk.ErrCode(codeLLMUpstreamError,
			fmt.Sprintf("model endpoint returned HTTP %d", resp.StatusCode))
	}

	reader := io.Reader(idleReader{reader: resp.Body, touch: touch})
	reader = &cappedReader{reader: reader, remaining: cfg.maxResponseBytes()}

	var text, reasoning strings.Builder
	toolCalls, usage, err := readSSE(reader, func(channel, delta string) error {
		if channel == "thinking" {
			reasoning.WriteString(delta)
		} else {
			text.WriteString(delta)
		}
		payload, _ := json.Marshal(chunkEvent{Channel: channel, Delta: delta})
		return p.sdk.EmitWithID(req.ID, "llm", "chunk", payload)
	})
	if err != nil {
		return nil, p.streamFailure(ctx, totalCtx, &idleExpired, url, err, idleTimeout)
	}

	out := &streamResp{
		Model: model.ID,
		Messages: []pluginsdk.ChatMessage{{
			Role:      pluginsdk.RoleAssistant,
			Text:      text.String(),
			Reasoning: reasoning.String(),
			ToolCalls: toolCalls,
		}},
	}
	if !usage.empty() {
		out.Usage = &usage
	}
	return out, nil
}

func (p *plugin) streamFailure(
	parent, total context.Context,
	idleExpired *atomic.Bool,
	url string,
	err error,
	idleTimeout time.Duration,
) error {
	if err := parent.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return pluginsdk.ErrCode(codeLLMTimeout, "model request timed out")
		}
		return err
	}
	if idleExpired.Load() {
		return pluginsdk.ErrCode(codeLLMTimeout,
			fmt.Sprintf("model stream idle for %s", idleTimeout))
	}
	if total.Err() != nil {
		return pluginsdk.ErrCode(codeLLMTimeout, "model request timed out")
	}
	if errors.Is(err, errResponseTooLarge) {
		return pluginsdk.ErrCode(codeLLMOutputLimit, "model response exceeded max_response_bytes")
	}
	fmt.Fprintf(os.Stderr, "llm-openai: %s: %v\n", url, err)
	return pluginsdk.ErrCode(codeLLMUpstreamError, "model endpoint request failed")
}

func buildChatRequest(model string, messages []pluginsdk.ChatMessage, tools []pluginsdk.Tool, maxTokens int) chatRequest {
	outTools := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		outTools = append(outTools, wireTool{
			Type:     "function",
			Function: tool,
		})
	}
	return chatRequest{
		Model:         model,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
		MaxTokens:     maxTokens,
		Messages:      wireMessages(messages),
		Tools:         outTools,
	}
}

func readSSE(r io.Reader, onDelta func(channel, delta string) error) ([]pluginsdk.ToolCall, usage, error) {
	// OpenAI sends the usage chunk last, with empty choices.
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	calls := make(map[int]*toolCallAccumulator)
	order := make([]int, 0)
	var lastUsage usage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, usage{}, fmt.Errorf("llm stream error: %s", chunk.Error.Message)
		}
		if !chunk.Usage.empty() {
			lastUsage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		reasoning := delta.ReasoningContent
		if reasoning == "" {
			reasoning = delta.Reasoning
		}
		if reasoning != "" {
			if err := onDelta("thinking", reasoning); err != nil {
				return nil, usage{}, err
			}
		}
		if delta.Content != "" {
			if err := onDelta("content", delta.Content); err != nil {
				return nil, usage{}, err
			}
		}
		for _, item := range delta.ToolCalls {
			index := len(calls)
			if item.Index != nil {
				index = *item.Index
			}
			call := calls[index]
			if call == nil {
				call = &toolCallAccumulator{index: index}
				calls[index] = call
				order = append(order, index)
			}
			if item.ID != "" {
				call.id = item.ID
			}
			if item.Type != "" {
				call.kind = item.Type
			}
			if item.Function.Name != "" {
				call.name = item.Function.Name
			}
			call.arguments.WriteString(item.Function.Arguments)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, usage{}, err
	}
	toolCalls, err := finalizeToolCalls(calls, order)
	if err != nil {
		return nil, usage{}, err
	}
	return toolCalls, lastUsage, nil
}

func finalizeToolCalls(calls map[int]*toolCallAccumulator, order []int) ([]pluginsdk.ToolCall, error) {
	sort.Ints(order)
	out := make([]pluginsdk.ToolCall, 0, len(calls))
	for _, index := range order {
		call := calls[index]
		if call == nil {
			continue
		}
		if strings.TrimSpace(call.name) == "" {
			return nil, fmt.Errorf("llm tool call %d has no function name", index)
		}
		rawArguments := strings.TrimSpace(call.arguments.String())
		if rawArguments == "" {
			rawArguments = "{}"
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rawArguments), &object); err != nil || object == nil {
			return nil, fmt.Errorf("llm tool call %s has invalid arguments: %s", call.id, rawArguments)
		}
		kind := call.kind
		if kind == "" {
			kind = "function"
		}
		id := call.id
		if id == "" {
			id = fmt.Sprintf("call-%d", index+1)
		}
		out = append(out, pluginsdk.ToolCall{
			Index: intPointer(index),
			ID:    id,
			Type:  kind,
			FunctionCall: pluginsdk.FunctionCall{
				Name:      call.name,
				Arguments: json.RawMessage(rawArguments),
			},
		})
	}
	return out, nil
}

func intPointer(value int) *int {
	return &value
}

func wireMessages(in []pluginsdk.ChatMessage) []wireMessage {
	out := make([]wireMessage, 0, len(in))
	for _, m := range in {
		message := wireMessage{
			Role:       m.Role,
			Content:    m.Text,
			ToolCallID: m.ToolCallID,
		}
		for _, call := range m.ToolCalls {
			arguments := toolArgumentsText(call.FunctionCall.Arguments)
			if arguments == "" {
				arguments = "{}"
			}
			kind := call.Type
			if kind == "" {
				kind = "function"
			}
			message.ToolCalls = append(message.ToolCalls, wireToolCall{
				ID:   call.ID,
				Type: kind,
				Function: wireFunctionCall{
					Name:      call.FunctionCall.Name,
					Arguments: arguments,
				},
			})
		}
		out = append(out, message)
	}
	return out
}

func toolArgumentsText(raw json.RawMessage) string {
	arguments := strings.TrimSpace(string(raw))
	if strings.HasPrefix(arguments, `"`) {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err == nil {
			return encoded
		}
	}
	return arguments
}
