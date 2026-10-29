package main

const (
	ActionSend     = "send"
	ActionFollowUp = "follow_up"
	ActionSteer    = "steer"
	ActionBtw      = "btw"
)

const ()

// fromWebUI 由前端发出的请求，包括 session 等相关信息
// 这些状态均由 session 启动过程发送到前端
type fromWebUI struct {
	Workspace string `json:"workspace"`
	SessionId string `json:"session_id"`
	UserInput string `json:"user_input"`

	Action string `json:"action"`
}

type sessionMeta struct {
	SessionId          string   `json:"session_id"`
	Title              string   `json:"title,omitempty"`
	Workspace          string   `json:"workspace"`
	ParentSessionId    string   `json:"parent_session_id,omitempty"`
	ChildrenSessionIds []string `json:"children_session_ids,omitempty"`
	Origin             string   `json:"origin"` // agent or subagent
	Pinned             bool     `json:"pinned,omitempty"`
	CreatedAt          int64    `json:"created_at"`
}

type sessionInfoReq struct {
	SessionId string `json:"session_id"`
}

type sessionInfoResp struct {
	sessionMeta
	FactCnt int `json:"fact_count"`
	NextSeq int `json:"next_seq"`
}

type sessionCreateReq struct {
	Title           string `json:"title"`
	Workspace       string `json:"workspace"`
	ParentSessionID string `json:"parent_session_id"`
	Origin          string `json:"origin"`
	Depth           int    `json:"depth"`
}

type chatReq struct {
}
