// 本来想 host（包括sdk在内）完全和 plugin 解耦，但一些结构体实在用到的地方太多了
// 不得不在 pluginsdk 定义这些结构体
// 也许还需要实现一些方法

package pluginsdk

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleToolCall  = "tool_call"
)

type ChatMessage struct {
}
