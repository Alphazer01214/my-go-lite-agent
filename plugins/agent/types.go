package agent

import (
	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// FromWebUI 由前端发出的请求，包括 session 等相关信息
// 这些状态均由 session 启动过程发送到前端
type FromWebUI struct {
	Workspace string `json:"workspace"`
	SessionId string `json:"session_id"`
}

type TurnReq struct {
	pluginsdk.ChatMessage
}
