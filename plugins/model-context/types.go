package main

import "encoding/json"

const (
	configFile = "config.json"

	defaultSystemPrompt = "You are LiteAgent, a local coding agent. Use tools when they are needed. " +
		"Never invent file contents. Keep file and command operations inside the workspace. " +
		"Report concrete results, errors and remaining risks."

	defaultMaxContextTokens   = 60000
	defaultCompactThreshold   = 0.8
	defaultRetainTokens       = 12000
	defaultCompactMaxChars    = 12000
	defaultAgentsMDMaxBytes   = 64 << 10
	defaultSnapshotLimit      = 128
	agentsMDName              = "AGENTS.md"
	runtimeContextHeader      = "Current runtime context. This snapshot supersedes earlier runtime-context snapshots."
	runtimeContextCleared     = "Current runtime context: none. Earlier runtime-context snapshots no longer apply."
	runtimeContextHeading     = "## "
	kindSystemPrompt          = "system_prompt"
	kindRuntimeContext        = "runtime_context"
	sourceModelContext        = "model-context"
	factTypeContextSnapshot   = "context_snapshot"
	factTypeContextSummary    = "context_summary"
	factTypeContextCompaction = "context_compaction"
	factTypeMessage           = "message"
	roleSystem                = "system"
	roleUser                  = "user"
	roleAssistant             = "assistant"
	roleTool                  = "tool"
	roleHost                  = "host"
)

type promptSection struct {
	Name  string `json:"name"`
	Order int    `json:"order,omitempty"`
	Text  string `json:"text"`
}

type config struct {
	PromptSections      []promptSection `json:"prompt_sections,omitempty"`
	SystemPrompt        string          `json:"system_prompt,omitempty"`
	MaxContextTokens    int             `json:"max_context_tokens,omitempty"`
	CompactThreshold    float64         `json:"compact_threshold,omitempty"`
	RetainRecentTokens  int             `json:"retain_recent_tokens,omitempty"`
	CompactMaxChars     int             `json:"compact_max_chars,omitempty"`
	AgentsMDMaxBytes    int             `json:"agents_md_max_bytes,omitempty"`
	SnapshotMemoryLimit int             `json:"snapshot_memory_limit,omitempty"`
}

type sectionAudit struct {
	Name  string `json:"name"`
	Order int    `json:"order,omitempty"`
	Hash  string `json:"hash"`
}

type completeMeta struct {
	SessionID     string `json:"session_id"`
	TurnID        string `json:"turn_id,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type llmMessage struct {
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

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Risk        int             `json:"risk,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
}

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
