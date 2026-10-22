package main

import (
	"encoding/json"
)

const (
	// chatWindowMessages bounds every ChatState payload. Older messages are
	// fetched through session.messages when the user asks for them.
	chatWindowMessages = 200
	chatWindowBytes    = 2 << 20
	// traceWindowFacts bounds the audit window. session.facts serves older pages.
	traceWindowFacts = 200
	traceWindowBytes = 2 << 20
	// uiPageLimit caps one explicit page read.
	uiPageLimit = 500
)

type uiPublisher interface {
	SetState(mountID string, state json.RawMessage) error
	SendCommand(mountID string, command string, payload json.RawMessage) error
}

type chatMessageUI struct {
	Seq        int             `json:"seq"`
	TurnID     string          `json:"turn_id,omitempty"`
	StepID     string          `json:"step_id,omitempty"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Reasoning  string          `json:"reasoning,omitempty"`
	Timestamp  int64           `json:"timestamp,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
	Source     string          `json:"source,omitempty"`
	Kind       string          `json:"kind,omitempty"`
}

type subagentUI struct {
	ChildSessionID string `json:"child_session_id"`
	State          string `json:"state"`
	StartedTS      int64  `json:"started_ts,omitempty"`
	EndedTS        int64  `json:"ended_ts,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	TurnCount      int    `json:"turn_count,omitempty"`
	Error          string `json:"error,omitempty"`
}

type chatTurnUI struct {
	TurnID          string `json:"turn_id,omitempty"`
	Status          string `json:"status"`
	Step            int    `json:"step,omitempty"`
	CancelRequested bool   `json:"cancel_requested"`
	Error           string `json:"error,omitempty"`
}

type chatTurnInfoUI struct {
	TurnID    string `json:"turn_id"`
	Status    string `json:"status"`
	Step      int    `json:"step,omitempty"`
	StartedAt int64  `json:"started_ts,omitempty"`
	EndedAt   int64  `json:"ended_ts,omitempty"`
	Messages  int    `json:"message_count"`
	Tools     int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type chatUsageUI struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Requests         int    `json:"requests"`
	LastPromptTokens int    `json:"last_prompt_tokens"`
	Model            string `json:"model,omitempty"`
	ContextWindow    int    `json:"context_window,omitempty"`
}

type chatContextUI struct {
	ModelContextID string   `json:"model_context_id,omitempty"`
	TurnID         string   `json:"turn_id,omitempty"`
	StepID         string   `json:"step_id,omitempty"`
	MessageCount   int      `json:"message_count"`
	TokenEstimate  int      `json:"token_estimate"`
	Model          string   `json:"model,omitempty"`
	ContextWindow  int      `json:"context_window,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

type chatRequestUI struct {
	TurnID        string `json:"turn_id,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type chatStateUI struct {
	SessionID       string           `json:"session_id"`
	Name            string           `json:"name,omitempty"`
	WorkspaceID     string           `json:"workspace_id,omitempty"`
	Workspace       string           `json:"workspace,omitempty"`
	Model           string           `json:"model,omitempty"`
	ContextWindow   int              `json:"context_window,omitempty"`
	Origin          string           `json:"origin,omitempty"`
	ParentSessionID string           `json:"parent_session_id,omitempty"`
	ReadOnly        bool             `json:"read_only,omitempty"`
	Subagents       []subagentUI     `json:"subagents,omitempty"`
	Revision        int              `json:"revision"`
	ThroughSeq      int              `json:"through_seq"`
	FirstSeq        int              `json:"first_seq,omitempty"`
	HasMore         bool             `json:"has_more,omitempty"`
	Messages        []chatMessageUI  `json:"messages"`
	Turn            chatTurnUI       `json:"turn"`
	Turns           []chatTurnInfoUI `json:"turns"`
	Usage           chatUsageUI      `json:"usage"`
	Context         *chatContextUI   `json:"context,omitempty"`
	Request         *chatRequestUI   `json:"request,omitempty"`
	Stream          *streamSnapshot  `json:"stream,omitempty"`
	Error           string           `json:"error,omitempty"`
}

type chatAppendUI struct {
	SessionID  string            `json:"session_id"`
	Revision   int               `json:"revision"`
	AfterSeq   int               `json:"after_seq"`
	ThroughSeq int               `json:"through_seq"`
	Messages   []chatMessageUI   `json:"messages"`
	Turn       *chatTurnAppendUI `json:"turn,omitempty"`
	Subagents  []subagentUI      `json:"subagents,omitempty"`
}

type chatTurnAppendUI struct {
	TurnID string `json:"turn_id,omitempty"`
	Status string `json:"status,omitempty"`
	Step   int    `json:"step,omitempty"`
	Error  string `json:"error,omitempty"`
}

type streamDeltaUI struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	StepID         string `json:"step_id"`
	UpdateID       int    `json:"update_id"`
	Reset          bool   `json:"reset"`
	ContentDelta   string `json:"content_delta,omitempty"`
	ReasoningDelta string `json:"reasoning_delta,omitempty"`
	Status         string `json:"status"`
}

type dockStateUI struct {
	SessionID     string         `json:"session_id,omitempty"`
	Name          string         `json:"name,omitempty"`
	WorkspaceID   string         `json:"workspace_id,omitempty"`
	Workspace     string         `json:"workspace,omitempty"`
	Revision      int            `json:"revision"`
	Model         string         `json:"model,omitempty"`
	ContextWindow int            `json:"context_window,omitempty"`
	Turn          chatTurnUI     `json:"turn"`
	Usage         chatUsageUI    `json:"usage"`
	Context       *chatContextUI `json:"context,omitempty"`
	Request       *chatRequestUI `json:"request,omitempty"`
}

type chatPanesStateUI struct {
	SelectedID  string        `json:"selected_id,omitempty"`
	WorkspaceID string        `json:"workspace_id,omitempty"`
	Revision    int           `json:"revision"`
	Panes       []chatStateUI `json:"panes"`
}

type historyStateUI struct {
	Sessions    []sessionSummary `json:"sessions"`
	SelectedID  string           `json:"selected_id,omitempty"`
	WorkspaceID string           `json:"workspace_id,omitempty"`
	// SelectedWorkspaceID is the registry's current workspace, which is not
	// necessarily WorkspaceID (that one follows the requested session). The
	// mount uses it to highlight the right workspace without a second call.
	SelectedWorkspaceID string            `json:"selected_workspace_id,omitempty"`
	Workspaces          []workspaceRecord `json:"workspaces"`
	OpenIDs             []string          `json:"open_ids,omitempty"`
	Revision            int               `json:"revision"`
}

type traceStateUI struct {
	SessionID  string `json:"session_id"`
	Revision   int    `json:"revision"`
	ThroughSeq int    `json:"through_seq"`
	FirstSeq   int    `json:"first_seq,omitempty"`
	HasMore    bool   `json:"has_more,omitempty"`
	Total      int    `json:"total"`
	Facts      []Fact `json:"facts"`
}

type traceAppendUI struct {
	SessionID  string `json:"session_id"`
	Revision   int    `json:"revision"`
	AfterSeq   int    `json:"after_seq"`
	ThroughSeq int    `json:"through_seq"`
	Total      int    `json:"total"`
	Facts      []Fact `json:"facts"`
}

type dockAppendUI struct {
	SessionID string            `json:"session_id"`
	Revision  int               `json:"revision"`
	Turn      *chatTurnAppendUI `json:"turn,omitempty"`
	Usage     *chatUsageUI      `json:"usage,omitempty"`
	Context   *chatContextUI    `json:"context,omitempty"`
	Request   *chatRequestUI    `json:"request,omitempty"`
}

type stateIn struct {
	SessionID string `json:"session_id,omitempty"`
}

type messagesIn struct {
	SessionID string `json:"session_id"`
	BeforeSeq int    `json:"before_seq,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type messagesOut struct {
	SessionID string          `json:"session_id"`
	Messages  []chatMessageUI `json:"messages"`
	FirstSeq  int             `json:"first_seq,omitempty"`
	HasMore   bool            `json:"has_more"`
	Total     int             `json:"total"`
}

type factsIn struct {
	SessionID string `json:"session_id"`
	BeforeSeq int    `json:"before_seq,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type factsOut struct {
	SessionID string `json:"session_id"`
	Facts     []Fact `json:"facts"`
	FirstSeq  int    `json:"first_seq,omitempty"`
	HasMore   bool   `json:"has_more"`
	Total     int    `json:"total"`
}

type stateOut struct {
	History historyStateUI `json:"history"`
	Chat    *chatStateUI   `json:"chat,omitempty"`
	Chats   []chatStateUI  `json:"chats,omitempty"`
	Trace   *traceStateUI  `json:"trace,omitempty"`
}

type selectIn struct {
	SessionID string `json:"session_id"`
}

type selectOut struct {
	SessionID string `json:"session_id"`
}

type closeIn struct {
	SessionID string `json:"session_id"`
}

type closeOut struct {
	SessionID string `json:"session_id"`
	Closed    bool   `json:"closed"`
	Selected  string `json:"selected_id,omitempty"`
}

type updateIn struct {
	SessionID   string  `json:"session_id"`
	Name        *string `json:"name,omitempty"`
	Workspace   *string `json:"workspace,omitempty"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
}

type updateOut struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

type factContextMeta struct {
	TurnID    string `json:"turn_id"`
	StepID    string `json:"step_id"`
	Reason    string `json:"reason"`
	Reasoning string `json:"reasoning"`
	Step      int    `json:"step"`
	Plugin    string `json:"plugin,omitempty"`
	Source    string `json:"source,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Error     string `json:"error,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}
