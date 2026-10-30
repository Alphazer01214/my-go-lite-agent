package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	body, err := json.Marshal(buildChatRequest(model.ID, in.Messages))
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

	var text strings.Builder
	err = readSSE(resp.Body, func(delta string) error {
		text.WriteString(delta)
		payload, _ := json.Marshal(chunkEvent{Channel: "content", Delta: delta})
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
			Role: pluginsdk.RoleAssistant,
			Text: text.String(),
		}},
	}, nil
}

func buildChatRequest(model string, messages []pluginsdk.ChatMessage) chatRequest {
	return chatRequest{
		Model:    model,
		Stream:   true,
		Messages: wireMessages(messages),
	}
}

func readSSE(r io.Reader, onDelta func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return fmt.Errorf("llm stream error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		if err := onDelta(chunk.Choices[0].Delta.Content); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func wireMessages(in []pluginsdk.ChatMessage) []wireMessage {
	out := make([]wireMessage, 0, len(in))
	for _, m := range in {
		out = append(out, wireMessage{Role: m.Role, Content: m.Text})
	}
	return out
}
