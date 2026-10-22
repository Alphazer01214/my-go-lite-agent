package main

import (
	"encoding/json"
)

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
	WorkspaceID      string `json:"workspace_id,omitempty"`
	IncludeSubagents bool   `json:"include_subagents,omitempty"`
}

type listOut struct {
	Sessions []sessionSummary `json:"sessions"`
}

type sessionSummary struct {
	SessionID        string `json:"session_id"`
	Name             string `json:"name,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	ParentSessionID  string `json:"parent_session_id,omitempty"`
	Origin           string `json:"origin,omitempty"`
	DelegationDepth  int    `json:"delegation_depth,omitempty"`
	Pinned           bool   `json:"pinned,omitempty"`
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
	Pinned           bool   `json:"pinned,omitempty"`
	CreatedTimestamp int64  `json:"created_ts"`
	FactCount        int    `json:"fact_count"`
	NextSeq          int    `json:"next_seq"`
}

type deleteIn struct {
	SessionID string `json:"session_id"`
}

type deleteOut struct {
	SessionID string `json:"session_id"`
	Deleted   bool   `json:"deleted"`
	Selected  string `json:"selected_id,omitempty"`
}
