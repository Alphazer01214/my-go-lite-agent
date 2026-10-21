package main

import (
	"bytes"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	codeDepthExceeded    = "subagent_depth_exceeded"
	codeParentUnresolved = "subagent_parent_unresolved"
	codeSpawnFailed      = "subagent_spawn_failed"
	codeToolsUnavailable = "subagent_tools_unavailable"
)

type providerInput struct {
	Arguments   json.RawMessage `json:"arguments"`
	SessionID   string          `json:"session_id,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Mode        string          `json:"mode,omitempty"`
}

func decodeProvider(raw json.RawMessage) (providerInput, json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return providerInput{}, nil, errBadArguments("payload is required")
	}
	var in providerInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return providerInput{}, nil, errBadArguments(err.Error())
	}
	args := in.Arguments
	if len(bytes.TrimSpace(args)) == 0 {
		// Direct capability calls may pass the arguments object at the top level.
		args = raw
	}
	return in, args, nil
}

type runIn struct {
	Prompt   string `json:"prompt"`
	MaxSteps int    `json:"max_steps,omitempty"`
}

type toolResult struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type turnIn struct {
	Input           string   `json:"input"`
	SessionID       string   `json:"session_id"`
	WorkspaceID     string   `json:"workspace_id,omitempty"`
	Workspace       string   `json:"workspace,omitempty"`
	Model           string   `json:"model,omitempty"`
	ContextWindow   int      `json:"context_window,omitempty"`
	MaxSteps        int      `json:"max_steps,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	ParentSessionID string   `json:"parent_session_id,omitempty"`
	Origin          string   `json:"origin,omitempty"`
	DelegationDepth int      `json:"delegation_depth,omitempty"`
}

type turnOut struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
	Steps     int    `json:"steps"`
	Cancelled bool   `json:"cancelled"`
	Usage     usage  `json:"usage"`
	Seqs      []int  `json:"seqs,omitempty"`
}

type sessionCreateIn struct {
	Name            string `json:"name,omitempty"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Model           string `json:"model,omitempty"`
	ContextWindow   int    `json:"context_window,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegation_depth,omitempty"`
}

type sessionCreateOut struct {
	SessionID string `json:"session_id"`
}

type sessionInfoOut struct {
	SessionID       string `json:"session_id"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Model           string `json:"model,omitempty"`
	ContextWindow   int    `json:"context_window,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegation_depth,omitempty"`
}

type factIn struct {
	Type       string          `json:"type"`
	Role       string          `json:"role,omitempty"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

type appendIn struct {
	SessionID string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type appendOut struct {
	SessionID string `json:"session_id"`
	Seqs      []int  `json:"seqs"`
	NextSeq   int    `json:"next_seq"`
}

type toolEntry struct {
	Name string `json:"name"`
}

type toolsListOut struct {
	Tools []toolEntry `json:"tools"`
}

type toolDefinition struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputSchema      json.RawMessage `json:"input_schema"`
	Risk             int             `json:"risk"`
	ParallelSafe     bool            `json:"parallel_safe,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
	TargetCapability string          `json:"target_capability"`
	TargetMethod     string          `json:"target_method"`
}

func mustJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return body
}

func errBadArguments(message string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, message)
}

func errCoded(code, message string) error {
	return pluginsdk.ErrCode(code, message)
}
