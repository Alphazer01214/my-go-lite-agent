package main

import "encoding/json"

// Fact is one append-only log line. ToolCalls/Meta stay opaque to session
// except context_summary meta fields read by projectMessages.
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

// Fact types in the append-only log. Some are written by this plugin and some
// only by the agent plugin (step_end, request_header, llm_usage); they are
// declared here as the single description of the log vocabulary, so a reader
// knows the full set even when this plugin never produces a given type.
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
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleHost      = "host"
)

// Meta is session metadata rebuilt from the session_start fact (cache only).
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
	CreatedTimestamp int64  `json:"created_ts"`
}

// summaryMeta is read from context_summary facts. ShadowedFromSeq/ToSeq define
// the ordinary-message range replaced by the summary; CoversThroughSeq keeps
// older summaries readable.
type summaryMeta struct {
	Active           bool `json:"active"`
	CoversThroughSeq int  `json:"covers_through_seq"`
	ShadowedFromSeq  int  `json:"shadowed_from_seq"`
	ShadowedToSeq    int  `json:"shadowed_to_seq"`
}

// messageMeta is the projection-relevant slice of a message fact's meta.
// model-context marks its system prompt and runtime snapshots here.
type messageMeta struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Hash   string `json:"hash"`
}

const (
	kindSystemPrompt   = "system_prompt"
	kindRuntimeContext = "runtime_context"
)

// chatMessage is derive output (OpenAI chat shape; type kept local).
// Seq and Meta are audit fields: LLM adapters ignore them, model-context uses
// them to identify its own facts.
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

// --- method payloads (Frame payload bodies) ---

type createIn struct {
	SessionID       string `json:"session_id,omitempty"`
	Name            string `json:"name,omitempty"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Model           string `json:"model,omitempty"`
	ContextWindow   int    `json:"context_window,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegation_depth,omitempty"`
}

type createOut struct {
	SessionID        string `json:"session_id"`
	CreatedTimestamp int64  `json:"created_ts"`
}

type appendIn struct {
	SessionID string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type factIn struct {
	Type       string          `json:"type"`
	Role       string          `json:"role,omitempty"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

type appendOut struct {
	SessionID string `json:"session_id"`
	Seqs      []int  `json:"seqs"`
	NextSeq   int    `json:"next_seq"`
}

type queryIn struct {
	SessionID string   `json:"session_id"`
	AfterSeq  int      `json:"after_seq,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Types     []string `json:"types,omitempty"`
}

type queryOut struct {
	Facts   []Fact `json:"facts"`
	NextSeq int    `json:"next_seq"`
	Total   int    `json:"total"`
}

type deriveIn struct {
	SessionID       string `json:"session_id"`
	FullToolResults *int   `json:"full_tool_results,omitempty"`
}

type deriveOut struct {
	Messages          []chatMessage `json:"messages"`
	CoveredThroughSeq int           `json:"covered_through_seq"`
	SummarySeq        int           `json:"summary_seq,omitempty"`
	TruncatedTools    int           `json:"truncated_tools"`
	TruncatedSeqs     []int         `json:"truncated_seqs,omitempty"`
}

type listIn struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
}

type listOut struct {
	Sessions []sessionSummary `json:"sessions"`
}

type sessionSummary struct {
	SessionID        string `json:"session_id"`
	Name             string `json:"name,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	FactCount        int    `json:"fact_count"`
	CreatedTimestamp int64  `json:"created_ts"`
	LastTimestamp    int64  `json:"last_ts"`
}

type infoIn struct {
	SessionID string `json:"session_id"`
}

type infoOut struct {
	SessionID        string `json:"session_id"`
	Name             string `json:"name,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	Model            string `json:"model,omitempty"`
	ContextWindow    int    `json:"context_window,omitempty"`
	ParentSessionID  string `json:"parent_session_id,omitempty"`
	Origin           string `json:"origin,omitempty"`
	DelegationDepth  int    `json:"delegation_depth,omitempty"`
	CreatedTimestamp int64  `json:"created_ts"`
	FactCount        int    `json:"fact_count"`
	NextSeq          int    `json:"next_seq"`
}

type workspaceListOut struct {
	Workspaces []workspaceRecord `json:"workspaces"`
	SelectedID string            `json:"selected_id,omitempty"`
}

type workspaceCreateIn struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type workspaceCreateOut struct {
	Workspace workspaceRecord `json:"workspace"`
	Created   bool            `json:"created"`
}

type workspaceSelectIn struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceSelectOut struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceRenameIn struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

type workspaceRenameOut struct {
	Workspace workspaceRecord `json:"workspace"`
}

type workspaceRemoveIn struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceRemoveOut struct {
	WorkspaceID string `json:"workspace_id"`
	Removed     bool   `json:"removed"`
	SelectedID  string `json:"selected_id,omitempty"`
}

type workspaceCurrentOut struct {
	Workspace *workspaceRecord `json:"workspace,omitempty"`
}

// plugin config (config.json)
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
