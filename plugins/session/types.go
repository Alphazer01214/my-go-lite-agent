package main

import (
	"encoding/json"
)

const (
	TypeSessionStart = "session_start"
	TypeTurnStart    = "turn_start"
	TypeTurnEnd      = "turn_end"
	TypeStepStart    = "step_start"
	TypeStepEnd      = "step_end"
)

// 磁盘记录的 kind。首行固定为 meta，其余为 fact 或 meta 修订行。
const (
	RecordKindMeta = "meta"
	RecordKindFact = "fact"
)

// Fact session 内部不解析 pluginsdk 提供的结构体，使用 json.RawMessage
type Fact struct {
	Seq         int             `json:"seq"`
	Type        string          `json:"type"`
	Role        string          `json:"role,omitempty"`
	TextContent string          `json:"text_content,omitempty"`
	Files       json.RawMessage `json:"files,omitempty"`
	ToolCalls   json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID  string          `json:"tool_call_id,omitempty"`
	// Meta 自定义的描述字段
	Meta      json.RawMessage `json:"meta,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

type Meta struct {
	SessionId          string   `json:"session_id"`
	Title              string   `json:"title,omitempty"`
	WorkspaceId        string   `json:"workspace_id,omitempty"`
	Workspace          string   `json:"workspace"`
	ParentSessionId    string   `json:"parent_session_id,omitempty"`
	ChildrenSessionIds []string `json:"children_session_ids,omitempty"`
	Origin             string   `json:"origin"` // agent or subagent
	Pinned             bool     `json:"pinned,omitempty"`
	CreatedAt          int64    `json:"created_at"`
}

// Workspace 是会话的分类依据；path 为规范化绝对路径，创建后不可修改。
type Workspace struct {
	WorkspaceId string `json:"workspace_id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	CreatedAt   int64  `json:"created_ts"`
	UpdatedAt   int64  `json:"updated_ts"`
}

// metaRecord 是 JSONL 中的元数据行；嵌入 Meta 使字段与 factRecord 平级。
type metaRecord struct {
	Kind string `json:"kind"`
	Meta
}

// factRecord 是 JSONL 中的事实行。
type factRecord struct {
	Kind string `json:"kind"`
	Fact
}

type infoReq struct {
	SessionId   string `json:"session_id"`
	Workspace   string `json:"workspace"`
	WorkspaceId string `json:"workspace_id"`
}

type infoResp struct {
	Meta
	FactCnt int `json:"fact_count"`
	NextSeq int `json:"next_seq"`
}

type createReq struct {
	Title           string `json:"title"`
	WorkspaceId     string `json:"workspace_id,omitempty"`
	Workspace       string `json:"workspace"`
	ParentSessionID string `json:"parent_session_id"`
	Origin          string `json:"origin"`
	Depth           int    `json:"depth"`
}

type appendReq struct {
	SessionId string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type factIn struct {
	Type        string          `json:"type"`
	Role        string          `json:"role,omitempty"`
	TextContent string          `json:"text_content,omitempty"`
	Files       json.RawMessage `json:"files,omitempty"`
	ToolCalls   json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID  string          `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage `json:"meta,omitempty"`
	// Timestamp 可选；为 0 时使用追加时的当前时间。
	Timestamp int64 `json:"timestamp,omitempty"`
}

type appendResp struct {
	SessionId string `json:"session_id"`
	Seqs      []int  `json:"seqs"`
	NextSeq   int    `json:"next_seq"`
}

type factsReq struct {
	SessionId string   `json:"session_id"`
	AfterSeq  int      `json:"after_seq,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Types     []string `json:"types,omitempty"`
}

type factsResp struct {
	Facts   []Fact `json:"facts"`
	NextSeq int    `json:"next_seq"`
	Total   int    `json:"total"`
}

// workspaceReq 是 workspace capability 的统一输入；各 method 只读取自己用到的字段。
type workspaceReq struct {
	WorkspaceId string `json:"workspace_id,omitempty"`
	Path        string `json:"path,omitempty"`
	Name        string `json:"name,omitempty"`
}

type workspaceListResp struct {
	Workspaces []Workspace `json:"workspaces"`
	SelectedId string      `json:"selected_id"`
}

type workspaceCreateResp struct {
	Workspace  Workspace `json:"workspace"`
	Created    bool      `json:"created"`
	SelectedId string    `json:"selected_id"`
}

type workspaceUpdateResp struct {
	Workspace Workspace `json:"workspace"`
}

type workspaceSelectResp struct {
	Workspace  Workspace `json:"workspace"`
	SelectedId string    `json:"selected_id"`
}

type workspaceDeleteResp struct {
	WorkspaceId string `json:"workspace_id"`
	Deleted     bool   `json:"deleted"`
	SelectedId  string `json:"selected_id"`
}

type workspaceCurrentResp struct {
	Workspace *Workspace `json:"workspace,omitempty"`
}

type listReq struct {
	WorkspaceId      string `json:"workspace_id,omitempty"`
	IncludeSubagents bool   `json:"include_subagents,omitempty"`
}

type listResp struct {
	WorkspaceId string           `json:"workspace_id"`
	Sessions    []sessionSummary `json:"sessions"`
}

type sessionSummary struct {
	Meta
	FactCnt int   `json:"fact_count"`
	NextSeq int   `json:"next_seq"`
	LastTs  int64 `json:"last_ts"`
}
