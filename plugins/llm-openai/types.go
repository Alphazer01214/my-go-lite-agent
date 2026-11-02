package main

import (
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type streamReq struct {
	Model    string                  `json:"model,omitempty"`
	Messages []pluginsdk.ChatMessage `json:"messages,omitempty"`
	Tools    []pluginsdk.Tool        `json:"tools,omitempty"`
}

type streamResp struct {
	Model    string                  `json:"model"`
	Messages []pluginsdk.ChatMessage `json:"messages"`
}

type chunkEvent struct {
	Channel string `json:"channel"`
	Delta   string `json:"delta"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireTool struct {
	Type     string         `json:"type"`
	Function pluginsdk.Tool `json:"function"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type toolCallAccumulator struct {
	index     int
	id        string
	kind      string
	name      string
	arguments strings.Builder
}
