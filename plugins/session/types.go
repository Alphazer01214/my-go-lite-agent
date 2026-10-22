package main

import (
	"encoding/json"
)

type Fact struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Role       string          `json:"role,omitempty"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
	Timestamp  int64           `json:"ts,omitempty"`
}

type Meta struct {
	SessionID        string `json:"session_id"`
	Name             string `json:"name,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	Model            string `json:"model,omitempty"`
	ContextWindow    int    `json:"context_window,omitempty"`
	ParentSessionID  string `json:"parent_session_id,omitempty"`
	Origin           string `json:"origin,omitempty"`
	DelegationDepth  int    `json:"delegation_depth,omitempty"`
	Pinned           bool   `json:"pinned,omitempty"`
	CreatedTimestamp int64  `json:"created_ts"`
}

type summaryMeta struct {
	Active           bool `json:"active"`
	CoversThroughSeq int  `json:"covers_through_seq"`
	ShadowedFromSeq  int  `json:"shadowed_from_seq"`
	ShadowedToSeq    int  `json:"shadowed_to_seq"`
}

type messageMeta struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Hash   string `json:"hash"`
}

type chatMessage struct {
	Seq        int             `json:"seq,omitempty"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

type streamMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Reasoning  string          `json:"reasoning,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type streamSnapshot struct {
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	StepID    string        `json:"step_id"`
	UpdateID  int           `json:"update_id"`
	Message   streamMessage `json:"message"`
	Status    string        `json:"status"`
}

type streamUpdateIn = streamSnapshot

type streamUpdateOut struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

type config struct {
	FullToolResults int `json:"fullToolResults"`
}

func defaultConfig() config {
	return config{FullToolResults: 8}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

const (
	TypeSessionStart      = "session_start"
	TypeTurnStart         = "turn_start"
	TypeTurnEnd           = "turn_end"
	TypeStepStart         = "step_start"
	TypeStepEnd           = "step_end"
	TypeRequestHeader     = "request_header"
	TypeLLMUsage          = "llm_usage"
	TypeMessage           = "message"
	TypeContextSummary    = "context_summary"
	TypeContextCompaction = "context_compaction"
	TypeContextSnapshot   = "context_snapshot"
	TypeSessionMeta       = "session_meta"
	// TypeSandboxDecision is written by the sandbox plugin. It carries no
	// message, so it never enters the model context.
	TypeSandboxDecision = "sandbox_decision"
	// Written by the subagent plugin: the parent-side start and finish markers.
	// Both are host facts and never enter model context.
	TypeSubagentSpawned  = "subagent_spawned"
	TypeSubagentFinished = "subagent_finished"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleHost      = "host"
)

const (
	kindSystemPrompt   = "system_prompt"
	kindRuntimeContext = "runtime_context"
)
