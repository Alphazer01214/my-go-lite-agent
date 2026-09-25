package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	chunkContent   = "content"
	chunkReasoning = "reasoning"
	// channel name for tool_call deltas (type is chunkToolCall in types.go)
	chunkToolCallChannel = "tool_call"
)

type streamEmitter func(payload any) error

// upstreamRequest is the OpenAI chat/completions body.
type upstreamRequest struct {
	Model         string        `json:"model"`
	Messages      []chatMessage `json:"messages"`
	Tools         []tool        `json:"tools,omitempty"`
	Stream        bool          `json:"stream,omitempty"`
	Temperature   *float64      `json:"temperature,omitempty"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type upstreamResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   string     `json:"content"`
			Reasoning string     `json:"reasoning_content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage usage `json:"usage"`
}

type upstreamDelta struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *usage `json:"usage"`
}

// emitFunc delivers one marshaled chunk payload (attributed evt).
type emitFunc func(payload json.RawMessage) error

func (p *plugin) complete(in completeIn, emit emitFunc) (completeOut, error) {
	if len(in.Messages) == 0 {
		return completeOut{}, errBadArguments("messages is required")
	}
	stream := true
	if in.Stream != nil {
		stream = *in.Stream
	}
	cfg := p.snapshot()
	model := cfg.Model
	if in.Model != "" {
		model = in.Model
	}
	req := upstreamRequest{
		Model:       model,
		Messages:    in.Messages,
		Tools:       in.Tools,
		Stream:      stream,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
	}
	p.beginWork()
	defer p.endWork()
	if stream {
		req.StreamOptions = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}
		return p.completeStream(cfg, req, emit)
	}
	return p.completeJSON(cfg, req)
}

func (p *plugin) completeJSON(cfg config, req upstreamRequest) (completeOut, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return completeOut{}, errHandler(err.Error())
	}
	httpReq, err := http.NewRequest(http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return completeOut{}, errUpstream(err.Error())
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		return completeOut{}, errUpstream(err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return completeOut{}, errUpstream(err.Error())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return completeOut{}, errUpstream(fmt.Sprintf("http %d: %s", resp.StatusCode, truncate(string(data), 200)))
	}
	var ur upstreamResponse
	if err := json.Unmarshal(data, &ur); err != nil {
		return completeOut{}, errUpstream(err.Error())
	}
	out := completeOut{Usage: ur.Usage}
	if len(ur.Choices) > 0 {
		ch := ur.Choices[0]
		out.Content = ch.Message.Content
		out.Reasoning = ch.Message.Reasoning
		out.ToolCalls = ch.Message.ToolCalls
		out.FinishReason = ch.FinishReason
	}
	return out, nil
}

func (p *plugin) completeStream(cfg config, req upstreamRequest, emit emitFunc) (completeOut, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return completeOut{}, errHandler(err.Error())
	}
	httpReq, err := http.NewRequest(http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return completeOut{}, errUpstream(err.Error())
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 0} // stream until server closes
	resp, err := client.Do(httpReq)
	if err != nil {
		return completeOut{}, errUpstream(err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return completeOut{}, errUpstream(fmt.Sprintf("http %d: %s", resp.StatusCode, truncate(string(data), 200)))
	}

	out := completeOut{}
	var content, reasoning strings.Builder
	toolsByIndex := map[int]*toolCall{}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var d upstreamDelta
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			continue
		}
		if d.Usage != nil {
			out.Usage = *d.Usage
		}
		for _, choice := range d.Choices {
			if choice.FinishReason != "" {
				out.FinishReason = choice.FinishReason
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				_ = emitJSON(emit, chunkText{Channel: chunkContent, Delta: choice.Delta.Content})
			}
			if choice.Delta.ReasoningContent != "" {
				reasoning.WriteString(choice.Delta.ReasoningContent)
				_ = emitJSON(emit, chunkText{Channel: chunkReasoning, Delta: choice.Delta.ReasoningContent})
			}
			for _, tc := range choice.Delta.ToolCalls {
				cur, ok := toolsByIndex[tc.Index]
				if !ok {
					cur = &toolCall{ID: tc.ID, Type: tc.Type}
					cur.Function.Name = tc.Function.Name
					toolsByIndex[tc.Index] = cur
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Type != "" {
					cur.Type = tc.Type
				}
				if tc.Function.Name != "" {
					cur.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					cur.Function.Arguments += tc.Function.Arguments
				}
				_ = emitJSON(emit, chunkToolCall{
					Channel: chunkToolCallChannel,
					Index:   tc.Index,
					ID:      tc.ID,
					Type:    tc.Type,
					Function: struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return completeOut{}, errUpstream(err.Error())
	}

	out.Content = content.String()
	out.Reasoning = reasoning.String()
	if len(toolsByIndex) > 0 {
		max := 0
		for i := range toolsByIndex {
			if i > max {
				max = i
			}
		}
		out.ToolCalls = make([]toolCall, 0, len(toolsByIndex))
		for i := 0; i <= max; i++ {
			if tc, ok := toolsByIndex[i]; ok {
				out.ToolCalls = append(out.ToolCalls, *tc)
			}
		}
	}
	if out.FinishReason == "" {
		if len(out.ToolCalls) > 0 {
			out.FinishReason = "tool_calls"
		} else {
			out.FinishReason = "stop"
		}
	}
	return out, nil
}

func emitJSON(emit emitFunc, payload any) error {
	if emit == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return emit(data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
