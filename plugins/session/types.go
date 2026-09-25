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

// Fact types (model-visible: message + active context_summary only).
const (
	TypeSessionStart    = "session_start"
	TypeTurnStart       = "turn_start"
	TypeTurnEnd         = "turn_end"
	TypeStepStart       = "step_start"
	TypeStepEnd         = "step_end"
	TypeRequestHeader   = "request_header"
	TypeLLMUsage        = "llm_usage"
	TypeMessage         = "message"
	TypeContextSummary  = "context_summary"
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
	Workspace        string `json:"workspace,omitempty"`
	ParentSessionID  string `json:"parent_session_id,omitempty"`
	Origin           string `json:"origin,omitempty"`
	DelegationDepth  int    `json:"delegation_depth,omitempty"`
	CreatedTimestamp int64  `json:"created_ts"`
}

// summaryMeta is the only meta slice session parses (derive projection).
type summaryMeta struct {
	Active           bool `json:"active"`
	CoversThroughSeq int  `json:"covers_through_seq"`
}

// chatMessage is derive output (OpenAI chat shape; type kept local).
type chatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// --- method payloads (Frame payload bodies) ---

type createIn struct {
	SessionID       string `json:"session_id,omitempty"`
	Name            string `json:"name,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
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
}

type listIn struct{}

type listOut struct {
	Sessions []sessionSummary `json:"sessions"`
}

type sessionSummary struct {
	SessionID        string `json:"session_id"`
	Name             string `json:"name,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	FactCount        int    `json:"fact_count"`
	CreatedTimestamp int64  `json:"created_ts"`
	LastTimestamp    int64  `json:"last_ts"`
}

type infoIn struct {
	SessionID string `json:"session_id"`
}

type infoOut struct {
	SessionID       string `json:"session_id"`
	Name            string `json:"name,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegation_depth,omitempty"`
	CreatedTimestamp int64  `json:"created_ts"`
	FactCount       int    `json:"fact_count"`
	NextSeq         int    `json:"next_seq"`
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
