package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	TypeSessionStart = "session_start"
	TypeTurnStart    = "turn_start"
	TypeTurnEnd      = "turn_end"
	TypeStepStart    = "step_start"
	TypeStepEnd      = "step_end"
	TypeMessage      = "message"
)

// 磁盘记录的 kind。首行固定为 meta，其余为 fact 或 meta 修订行。
const (
	RecordKindMeta = "meta"
	RecordKindFact = "fact"
)

type Fact struct {
	Seq         int                  `json:"seq"`
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	// Meta 自定义的描述字段
	Meta      json.RawMessage `json:"meta,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

type Meta struct {
	SessionId          string   `json:"session_id"`
	Title              string   `json:"title,omitempty"`
	Workspace          string   `json:"workspace"`
	ParentSessionId    string   `json:"parent_session_id,omitempty"`
	ChildrenSessionIds []string `json:"children_session_ids,omitempty"`
	Origin             string   `json:"origin"` // agent or subagent
	Model              string   `json:"model,omitempty"`
	Pinned             bool     `json:"pinned,omitempty"`
	CreatedAt          int64    `json:"created_at"`
}

// Workspace 是会话的分类依据；path 为规范化绝对路径，既是唯一标识也是唯一入参，创建后不可修改。
type Workspace struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_ts"`
	UpdatedAt int64  `json:"updated_ts"`
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
	SessionId string `json:"session_id"`
}

type infoResp struct {
	Meta
	//Facts   []Fact `json:"facts"`
	FactCnt int `json:"fact_count"`
	NextSeq int `json:"next_seq"`
}

type messageResp struct {
	Messages []pluginsdk.ChatMessage `json:"messages"`
}

type createReq struct {
	Title           string `json:"title"`
	Workspace       string `json:"workspace"`
	ParentSessionID string `json:"parent_session_id"`
	Origin          string `json:"origin"`
	Model           string `json:"model,omitempty"`
	Depth           int    `json:"depth"`
	// Facts 是可选的初始事实，由调用方提供；session 只按序记录，不解释含义。
	Facts []factIn `json:"facts,omitempty"`
}

type appendReq struct {
	SessionId string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type factIn struct {
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage      `json:"meta,omitempty"`
	// Timestamp 可选；为 0 时使用追加时的当前时间。
	Timestamp int64 `json:"timestamp,omitempty"`
}

type appendResp struct {
	SessionId string `json:"session_id"`
	Seqs      []int  `json:"seqs"`
	NextSeq   int    `json:"next_seq"`
}

type updateReq struct {
	SessionId string  `json:"session_id"`
	Title     *string `json:"title,omitempty"`
	Pinned    *bool   `json:"pinned,omitempty"`
}

type updateResp struct {
	Meta
}

type deleteReq struct {
	SessionId string `json:"session_id"`
}

type deleteResp struct {
	SessionId string `json:"session_id"`
	Deleted   bool   `json:"deleted"`
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
	Path string `json:"path,omitempty"`
	Name string `json:"name,omitempty"`
}

type workspaceListResp struct {
	Workspaces []Workspace `json:"workspaces"`
	Selected   string      `json:"selected"`
}

type workspaceCreateResp struct {
	Workspace Workspace `json:"workspace"`
	Created   bool      `json:"created"`
	Selected  string    `json:"selected"`
}

type workspaceUpdateResp struct {
	Workspace Workspace `json:"workspace"`
}

type workspaceSelectResp struct {
	Workspace Workspace `json:"workspace"`
	Selected  string    `json:"selected"`
}

type workspaceDeleteResp struct {
	Path     string `json:"path"`
	Deleted  bool   `json:"deleted"`
	Selected string `json:"selected"`
}

type workspaceCurrentResp struct {
	Workspace *Workspace `json:"workspace,omitempty"`
}

type listReq struct {
	Workspace        string `json:"workspace,omitempty"`
	IncludeSubagents bool   `json:"include_subagents,omitempty"`
}

type listResp struct {
	Workspace string           `json:"workspace"`
	Sessions  []sessionSummary `json:"sessions"`
}

type sessionSummary struct {
	Meta
	FactCnt int   `json:"fact_count"`
	NextSeq int   `json:"next_seq"`
	LastTs  int64 `json:"last_ts"`
}
