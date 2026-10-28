package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

const (
	ActionSend     = "send"
	ActionFollowUp = "follow_up"
	ActionSteer    = "steer"
	ActionBtw      = "btw"
)

const ()

// FromWebUI 由前端发出的请求，包括 session 等相关信息
// 这些状态均由 session 启动过程发送到前端
type FromWebUI struct {
	Workspace string `json:"workspace"`
	SessionId string `json:"session_id"`
	UserInput string `json:"user_input"`

	Action string `json:"action"`
}

type SessionInfo struct {
	SessionId string `json:"session_id"`
	Workspace string `json:"workspace"`
	CreatedAt int64  `json:"created_at"`

	Messages []pluginsdk.ChatMessage `json:"messages"`
	//Facts []Fact `json:"facts"`
}

type TurnResponse struct {
	SessionId string `json:"session_id"`
	Accepted  bool   `json:"accepted"`
	Message   string `json:"message,omitempty"`
}
