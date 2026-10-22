package main

import (
	"encoding/json"
)

type snapshotPart struct {
	Source string `json:"source"`
	Seq    int    `json:"seq,omitempty"`
	Seqs   []int  `json:"seqs,omitempty"`
}

type snapshot struct {
	ModelContextID string           `json:"model_context_id"`
	Revision       int              `json:"revision"`
	SessionID      string           `json:"session_id"`
	TurnID         string           `json:"turn_id,omitempty"`
	StepID         string           `json:"step_id,omitempty"`
	Mode           string           `json:"mode,omitempty"`
	Model          string           `json:"model,omitempty"`
	ContextWindow  int              `json:"context_window,omitempty"`
	Surface        []int            `json:"surface"`
	MessageCount   int              `json:"message_count"`
	MessagesHash   string           `json:"messages_hash"`
	TruncatedSeqs  []int            `json:"truncated_seqs,omitempty"`
	Tools          []toolDefinition `json:"tools"`
	ToolsHash      string           `json:"tools_hash"`
	TokenEstimate  int              `json:"token_estimate"`
	Parts          []snapshotPart   `json:"parts,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
}

type buildIn struct {
	SessionID        string           `json:"session_id"`
	TurnID           string           `json:"turn_id,omitempty"`
	StepID           string           `json:"step_id,omitempty"`
	Mode             string           `json:"mode,omitempty"`
	Scheme           string           `json:"scheme,omitempty"`
	Workspace        string           `json:"workspace,omitempty"`
	WorkspaceID      string           `json:"workspace_id,omitempty"`
	Model            string           `json:"model,omitempty"`
	ContextWindow    int              `json:"context_window,omitempty"`
	Tools            []toolDefinition `json:"tools,omitempty"`
	MaxContextTokens int              `json:"max_context_tokens,omitempty"`
}

type buildOut struct {
	ModelContextID     string           `json:"model_context_id"`
	Messages           []llmMessage     `json:"messages"`
	Tools              []toolDefinition `json:"tools"`
	TokenEstimate      int              `json:"token_estimate"`
	CompactNeeded      bool             `json:"compact_needed"`
	Warnings           []string         `json:"warnings,omitempty"`
	SystemPromptHash   string           `json:"system_prompt_hash"`
	RuntimeContextHash string           `json:"runtime_context_hash"`
}

type inspectIn struct {
	ModelContextID string `json:"model_context_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
}

type inspectedMessage struct {
	Seq           int             `json:"seq"`
	Role          string          `json:"role"`
	Content       string          `json:"content"`
	ToolCalls     json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID    string          `json:"tool_call_id,omitempty"`
	Source        string          `json:"source"`
	Truncated     bool            `json:"truncated,omitempty"`
	TokenEstimate int             `json:"token_estimate"`
}

type inspectBreakdown struct {
	SystemPrompt   int `json:"system_prompt_tokens"`
	RuntimeContext int `json:"runtime_context_tokens"`
	Summary        int `json:"summary_tokens"`
	Session        int `json:"session_tokens"`
	Tools          int `json:"tools_tokens"`
}

type inspectOut struct {
	Found     bool               `json:"found"`
	Snapshot  snapshot           `json:"snapshot"`
	Messages  []inspectedMessage `json:"messages"`
	Breakdown inspectBreakdown   `json:"breakdown"`
	Integrity string             `json:"integrity,omitempty"`
	Warnings  []string           `json:"warnings,omitempty"`
}

type compactIn struct {
	SessionID        string `json:"session_id"`
	KeepRecentTokens int    `json:"keep_recent_tokens,omitempty"`
	MaxSummaryChars  int    `json:"max_summary_chars,omitempty"`
}

type compactOut struct {
	SessionID     string `json:"session_id"`
	Compacted     bool   `json:"compacted"`
	SourceFromSeq int    `json:"source_from_seq,omitempty"`
	SourceToSeq   int    `json:"source_to_seq,omitempty"`
	SummarySeq    int    `json:"summary_seq,omitempty"`
	Messages      int    `json:"messages_covered"`
	TokenBefore   int    `json:"token_before"`
	TokenAfter    int    `json:"token_after"`
}

type assembly struct {
	SystemPrompt    string
	SystemHash      string
	SystemSections  []sectionAudit
	RuntimeContext  string
	RuntimeHash     string
	RuntimeSections []sectionAudit
	RuntimeCleared  bool
	Warnings        []string
}

type prepared struct {
	Messages []llmMessage
	Tools    []toolDefinition
	View     deriveOut
	Snapshot snapshot
	Warnings []string
}
