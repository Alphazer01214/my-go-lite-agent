package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const codeLLMUpstreamError = "llm_upstream_error"

func (p *plugin) stream(ctx context.Context, req *pluginsdk.Request, in *streamReq) (*streamResp, error) {
	if len(in.Messages) == 0 {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "messages is required")
	}
	cfg := p.snapshotConfig()
	model, err := cfg.model(in.Model)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(buildChatRequest(model.ID, in.Messages, in.Tools))
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(model.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, pluginsdk.ErrCode(codeLLMUpstreamError,
			fmt.Sprintf("llm upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(detail))))
	}

	var text, reasoning strings.Builder
	toolCalls, err := readSSE(resp.Body, func(channel, delta string) error {
		if channel == "thinking" {
			reasoning.WriteString(delta)
		} else {
			text.WriteString(delta)
		}
		payload, _ := json.Marshal(chunkEvent{Channel: channel, Delta: delta})
		return p.sdk.EmitWithID(req.ID, "llm", "chunk", payload)
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, pluginsdk.ErrCode(codeLLMUpstreamError, err.Error())
	}

	return &streamResp{
		Model: model.ID,
		Messages: []pluginsdk.ChatMessage{{
			Role:      pluginsdk.RoleAssistant,
			Text:      text.String(),
			Reasoning: reasoning.String(),
			ToolCalls: toolCalls,
		}},
	}, nil
}

func buildChatRequest(model string, messages []pluginsdk.ChatMessage, tools []pluginsdk.Tool) chatRequest {
	outTools := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		outTools = append(outTools, wireTool{
			Type:     "function",
			Function: tool,
		})
	}
	return chatRequest{
		Model:    model,
		Stream:   true,
		Messages: wireMessages(messages),
		Tools:    outTools,
	}
}

func readSSE(r io.Reader, onDelta func(channel, delta string) error) ([]pluginsdk.ToolCall, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	calls := make(map[int]*toolCallAccumulator)
	order := make([]int, 0)
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
			return nil, fmt.Errorf("llm stream error: %s", chunk.Error.Message)
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
				return nil, err
			}
		}
		if delta.Content != "" {
			if err := onDelta("content", delta.Content); err != nil {
				return nil, err
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
		return nil, err
	}
	return finalizeToolCalls(calls, order)
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
