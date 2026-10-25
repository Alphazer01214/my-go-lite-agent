// 本来想 host（包括sdk在内）完全和 plugin 解耦，但一些结构体实在用到的地方太多了
// 不得不在 pluginsdk 定义这些结构体
// 也许还需要实现一些方法

// messages.go 定义通用消息结构体，各插件取所需，可以直接复用，也可以自定义
// 暂时只实现文本+图像模态的

package pluginsdk

import "encoding/json"

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
	RoleDeveloper = "developer"
)

const (
	ContentText       = "text"
	ContentImage      = "image"
	ContentAudio      = "audio"
	ContentVideo      = "video"
	ContentFile       = "file"
	ContentToolCall   = "tool_call"
	ContentToolResult = "tool_result"

	ContentThinking = "thinking"
	ContentRefusal  = "refusal"
)

// ChatMessage needs to be compatible with text/image/audio...
type ChatMessage struct {
	ID    string `json:"id,omitempty"`
	Seq   int    `json:"seq,omitempty"`
	Role  string `json:"role"`
	Text  string `json:"text"`
	Files []File `json:"files,omitempty"`

	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Timestamp  int64      `json:"timestamp,omitempty"`
}

type ContentPart struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`
}

type File struct {
	URL  string `json:"url"`
	Data []byte `json:"data,omitempty"`
	Mime string `json:"mime,omitempty"`
	Name string `json:"name"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolCall struct {
	Index        *int         `json:"index,omitempty"` // streaming
	ID           string       `json:"id"`
	Type         string       `json:"type"` // function
	FunctionCall FunctionCall `json:"function_call,omitempty"`
}

type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
