package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

type streamReq struct {
	Model    string                  `json:"model,omitempty"`
	Prompt   string                  `json:"prompt,omitempty"`
	Messages []pluginsdk.ChatMessage `json:"messages,omitempty"`
}

type streamResp struct {
	Model string `json:"model,omitempty"`
	Text  string `json:"text"`
}
