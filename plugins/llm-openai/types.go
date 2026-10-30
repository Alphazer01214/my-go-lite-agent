package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

type streamReq struct {
	Model    string                  `json:"model,omitempty"`
	Messages []pluginsdk.ChatMessage `json:"messages,omitempty"`
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
}

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
