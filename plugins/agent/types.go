package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	ActionSend     = "send"
	ActionFollowUp = "follow_up"
	ActionSteer    = "steer"
	ActionBtw      = "btw"
)

const (
	codeSessionNotFound = "session_not_found"
	codeTurnBusy        = "turn_busy"
	factTypeMessage     = "message"
	factTypeTurnStart   = "turn_start"
	factTypeTurnEnd     = "turn_end"
)

// fromWebUI 由前端发出的请求，包括 session 等相关信息
// 这些状态均由 session 启动过程发送到前端
type fromWebUI struct {
	Workspace string `json:"workspace"`
	SessionId string `json:"session_id"`
	UserInput string `json:"user_input"`
	Model     string `json:"model,omitempty"`

	Action string `json:"action"`
}

type sessionMeta struct {
	SessionId          string   `json:"session_id"`
	Title              string   `json:"title,omitempty"`
	Workspace          string   `json:"workspace"`
	ParentSessionId    string   `json:"parent_session_id,omitempty"`
	ChildrenSessionIds []string `json:"children_session_ids,omitempty"`
	Origin             string   `json:"origin"` // agent or subagent
	Model              string   `json:"model,omitempty"`
	Pinned             bool     `json:"pinned,omitempty"`
	CreatedAt          int64    `json:"created_at"`
}

type sessionInfoReq struct {
	SessionId string `json:"session_id"`
}

type sessionInfoResp struct {
	sessionMeta
	FactCnt int `json:"fact_count"`
	NextSeq int `json:"next_seq"`
}

type sessionCreateReq struct {
	Title           string `json:"title"`
	Workspace       string `json:"workspace"`
	ParentSessionID string `json:"parent_session_id"`
	Origin          string `json:"origin"`
	Model           string `json:"model,omitempty"`
	Depth           int    `json:"depth"`
}

type sessionFact struct {
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage      `json:"meta,omitempty"`
}

type sessionAppendReq struct {
	SessionId string        `json:"session_id"`
	Facts     []sessionFact `json:"facts"`
}

type sessionAppendResp struct {
	Seqs []int `json:"seqs"`
}

type sessionMessagesResp struct {
	Messages []pluginsdk.ChatMessage `json:"messages"`
}

type llmStreamReq struct {
	Model    string                  `json:"model,omitempty"`
	Messages []pluginsdk.ChatMessage `json:"messages"`
}

type llmStreamResp struct {
	Model    string                  `json:"model"`
	Messages []pluginsdk.ChatMessage `json:"messages"`
}

type turnResp struct {
	SessionId string                  `json:"session_id"`
	Model     string                  `json:"model,omitempty"`
	Messages  []pluginsdk.ChatMessage `json:"messages"`
	Seqs      []int                   `json:"seqs"`
}

type loopStartedEvent struct {
	SessionId string `json:"session_id"`
	TurnId    string `json:"turn_id"`
}

type cancelReq struct {
	SessionId string `json:"session_id"`
}

type cancelResp struct {
	SessionId string `json:"session_id"`
	Cancelled bool   `json:"cancelled"`
}

type turnStartMeta struct {
	TurnId    string `json:"turn_id"`
	Workspace string `json:"workspace,omitempty"`
	Model     string `json:"model,omitempty"`
}

type turnEndMeta struct {
	TurnId     string `json:"turn_id"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

type chatReq struct {
}
