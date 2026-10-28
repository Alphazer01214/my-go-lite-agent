package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

type CompletionRequest struct {
	Model    string                  `json:"model,omitempty"`
	Prompt   string                  `json:"prompt,omitempty"`
	Messages []pluginsdk.ChatMessage `json:"messages,omitempty"`
}

type CompletionResponse struct {
	Model string `json:"model,omitempty"`
	Text  string `json:"text"`
}
