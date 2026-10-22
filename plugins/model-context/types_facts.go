package main

import (
	"encoding/json"
)

type derivedMessage struct {
	Seq        int             `json:"seq,omitempty"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

type derivedMeta struct {
	Source   string         `json:"source"`
	Kind     string         `json:"kind"`
	Hash     string         `json:"hash"`
	Sections []sectionAudit `json:"sections,omitempty"`
	Cleared  bool           `json:"cleared,omitempty"`
}

type deriveOut struct {
	Messages          []derivedMessage `json:"messages"`
	CoveredThroughSeq int              `json:"covered_through_seq"`
	SummarySeq        int              `json:"summary_seq,omitempty"`
	TruncatedTools    int              `json:"truncated_tools"`
	TruncatedSeqs     []int            `json:"truncated_seqs,omitempty"`
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

type fact struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
	Timestamp  int64           `json:"ts,omitempty"`
}

type queryIn struct {
	SessionID string   `json:"session_id"`
	Types     []string `json:"types,omitempty"`
}

type queryOut struct {
	Facts []fact `json:"facts"`
	Total int    `json:"total"`
}

type systemFactMeta struct {
	Plugin   string         `json:"plugin"`
	Source   string         `json:"source"`
	Kind     string         `json:"kind"`
	Hash     string         `json:"hash"`
	Sections []sectionAudit `json:"sections,omitempty"`
}

type runtimeFactMeta struct {
	Plugin   string         `json:"plugin"`
	Source   string         `json:"source"`
	Kind     string         `json:"kind"`
	Hash     string         `json:"hash"`
	Sections []sectionAudit `json:"sections,omitempty"`
	Cleared  bool           `json:"cleared,omitempty"`
}

type summaryFactMeta struct {
	Active           bool   `json:"active"`
	CoversThroughSeq int    `json:"covers_through_seq"`
	ShadowedFromSeq  int    `json:"shadowed_from_seq"`
	ShadowedToSeq    int    `json:"shadowed_to_seq"`
	PreviousSummary  int    `json:"previous_summary_seq,omitempty"`
	Plugin           string `json:"plugin"`
	GeneratedBy      string `json:"generated_by"`
	Strategy         string `json:"strategy"`
	Reason           string `json:"reason"`
	CompactionID     string `json:"compaction_id,omitempty"`
	MessagesCovered  int    `json:"messages_covered"`
}

type compactionFactMeta struct {
	Plugin          string `json:"plugin"`
	Reason          string `json:"reason"`
	Strategy        string `json:"strategy"`
	CompactionID    string `json:"compaction_id,omitempty"`
	SourceFromSeq   int    `json:"source_from_seq"`
	SourceToSeq     int    `json:"source_to_seq"`
	Messages        int    `json:"messages"`
	TokenBefore     int    `json:"token_before"`
	TokenAfter      int    `json:"token_after"`
	PreviousSummary int    `json:"previous_summary_seq,omitempty"`
}
