package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	factTypeMessage         = "message"
	factTypeSystemPrompt    = "system_prompt"
	factTypeRuntimeContext  = "runtime_context"
	factTypeContextSnapshot = "context_snapshot"

	trackStatic  = "static"
	trackDynamic = "dynamic"

	configPath = "config.json"
)

type promptSection struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
	Track string `json:"track,omitempty"`
	Text  string `json:"text"`
}

type skillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type mcpEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

type config struct {
	PromptSections []promptSection `json:"prompt_sections"`
	Skills         []skillEntry    `json:"skills,omitempty"`
	MCPServers     []mcpEntry      `json:"mcp_servers,omitempty"`
}

type createPayload struct {
	Workspace string `json:"workspace"`
	Model     string `json:"model,omitempty"`
}

type streamPayload struct {
	Model string               `json:"model,omitempty"`
	Meta  streamPayloadSession `json:"meta,omitempty"`
}

type streamPayloadSession struct {
	SessionID string `json:"session_id,omitempty"`
}

type sessionInfo struct {
	SessionID string `json:"session_id"`
	Workspace string `json:"workspace"`
	Model     string `json:"model,omitempty"`
}

type factIn struct {
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage      `json:"meta,omitempty"`
}

type sessionFact struct {
	Seq         int                  `json:"seq"`
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage      `json:"meta,omitempty"`
	Timestamp   int64                `json:"timestamp"`
}

type factsReq struct {
	SessionID string   `json:"session_id"`
	Types     []string `json:"types,omitempty"`
}

type factsResp struct {
	Facts   []sessionFact `json:"facts"`
	NextSeq int           `json:"next_seq"`
	Total   int           `json:"total"`
}

type appendReq struct {
	SessionID string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type appendResp struct {
	SessionID string `json:"session_id"`
	Seqs      []int  `json:"seqs"`
	NextSeq   int    `json:"next_seq"`
}

type factMeta struct {
	Source string `json:"source"`
	Hash   string `json:"hash,omitempty"`
}

type snapshotMeta struct {
	Source             string `json:"source"`
	SessionID          string `json:"session_id"`
	Model              string `json:"model,omitempty"`
	SurfaceSeqs        []int  `json:"surface_seqs,omitempty"`
	SystemPromptHash   string `json:"system_prompt_hash,omitempty"`
	RuntimeContextHash string `json:"runtime_context_hash,omitempty"`
	MessagesHash       string `json:"messages_hash,omitempty"`
	TokenEstimate      int    `json:"token_estimate"`
	MessageCount       int    `json:"message_count"`
}
