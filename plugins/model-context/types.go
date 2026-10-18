package main

import "encoding/json"

type config struct {
	SystemPrompt    string `json:"system_prompt"`
	MaxContextChars int    `json:"max_context_chars"`
}

func defaultConfig() config {
	return config{
		SystemPrompt: "You are LiteAgent, a local coding agent. Use tools when they are needed. " +
			"Never invent file contents. Keep file and command operations inside the workspace. " +
			"Report concrete results, errors and remaining risks.",
		MaxContextChars: 240000,
	}
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type buildIn struct {
	SessionID    string   `json:"session_id"`
	TurnID       string   `json:"turn_id,omitempty"`
	StepID       string   `json:"step_id,omitempty"`
	Scheme       string   `json:"scheme,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
	DeniedTools  []string `json:"denied_tools,omitempty"`
	MaxChars     *int     `json:"max_chars,omitempty"`
}

type snapshotPart struct {
	Source string `json:"source"`
	Bytes  int    `json:"bytes"`
}

type snapshot struct {
	ModelContextID string           `json:"model_context_id"`
	SessionID      string           `json:"session_id"`
	TurnID         string           `json:"turn_id,omitempty"`
	StepID         string           `json:"step_id,omitempty"`
	Revision       int              `json:"revision"`
	Messages       []chatMessage    `json:"messages"`
	Tools          []toolDefinition `json:"tools"`
	Parts          []snapshotPart   `json:"parts"`
	TokenEstimate  int              `json:"token_estimate"`
	Hash           string           `json:"hash"`
	Warnings       []string         `json:"warnings,omitempty"`
}

type buildOut struct {
	ModelContextID string           `json:"model_context_id"`
	Messages       []chatMessage    `json:"messages"`
	Tools          []toolDefinition `json:"tools"`
	TokenEstimate  int              `json:"token_estimate"`
	Warnings       []string         `json:"warnings,omitempty"`
}

type inspectIn struct {
	ModelContextID string `json:"model_context_id"`
}

type compactIn struct {
	SessionID string `json:"session_id"`
	KeepLast  *int   `json:"keep_last,omitempty"`
	MaxChars  *int   `json:"max_chars,omitempty"`
}

type compactOut struct {
	SessionID  string `json:"session_id"`
	Compacted  bool   `json:"compacted"`
	SourceFrom int    `json:"source_from_seq,omitempty"`
	SourceTo   int    `json:"source_to_seq,omitempty"`
	SummarySeq int    `json:"summary_seq,omitempty"`
	Messages   int    `json:"messages_covered"`
}

type deriveIn struct {
	SessionID string `json:"session_id"`
}

type deriveOut struct {
	Messages []chatMessage `json:"messages"`
}

type toolsListOut struct {
	Tools []toolSpec `json:"tools"`
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Risk        string          `json:"risk"`
}

type appendIn struct {
	SessionID string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type factIn struct {
	Type    string          `json:"type"`
	Role    string          `json:"role,omitempty"`
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

type appendOut struct {
	Seqs []int `json:"seqs"`
}

type fact struct {
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

type queryIn struct {
	SessionID string   `json:"session_id"`
	Types     []string `json:"types,omitempty"`
}

type queryOut struct {
	Facts []fact `json:"facts"`
	Total int    `json:"total"`
}
