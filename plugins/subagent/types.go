package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	codeParentUnresolved = "subagent_parent_unresolved"
	codeDepthExceeded    = "subagent_depth_exceeded"
	codeSpawnFailed      = "subagent_spawn_failed"

	factTypeSpawned  = "subagent_spawned"
	factTypeFinished = "subagent_finished"

	maxReplyBytes = 64 * 1024
)

type providerInput struct {
	Arguments json.RawMessage `json:"arguments,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

type runIn struct {
	Prompt string `json:"prompt"`
}

type toolResult struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type infoResponse struct {
	Capability string     `json:"capability"`
	Tools      []infoTool `json:"tools"`
}

type infoTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Method      string          `json:"method"`
}

type sessionInfo struct {
	SessionId       string `json:"session_id"`
	Workspace       string `json:"workspace"`
	ParentSessionId string `json:"parent_session_id"`
	Origin          string `json:"origin"`
	Model           string `json:"model"`
}

type sessionCreateReq struct {
	Title           string `json:"title"`
	Workspace       string `json:"workspace"`
	ParentSessionID string `json:"parent_session_id"`
	Origin          string `json:"origin"`
	Model           string `json:"model,omitempty"`
}

type sessionFact struct {
	Type        string          `json:"type"`
	Role        string          `json:"role,omitempty"`
	TextContent string          `json:"text_content,omitempty"`
	Meta        json.RawMessage `json:"meta,omitempty"`
}

type appendReq struct {
	SessionID string        `json:"session_id"`
	Facts     []sessionFact `json:"facts"`
}

type appendResp struct {
	Seqs []int `json:"seqs"`
}

type turnReq struct {
	Workspace string `json:"workspace"`
	SessionID string `json:"session_id"`
	UserInput string `json:"user_input"`
	Model     string `json:"model,omitempty"`
}

type turnResp struct {
	SessionID string                  `json:"session_id"`
	Model     string                  `json:"model,omitempty"`
	Messages  []pluginsdk.ChatMessage `json:"messages"`
	Seqs      []int                   `json:"seqs"`
}

type settingsValues struct {
	MaxDepth int `json:"max_depth"`
}

type settingsGetResp struct {
	Values settingsValues `json:"values"`
}

type settingsSaveReq struct {
	Values settingsValues `json:"values"`
}

type settingsSaveResp struct {
	Saved bool `json:"saved"`
}
