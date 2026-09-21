package host

import (
	"fmt"
	"time"
)

// 错误一律以 error 表达：
//   - 线格式用 Code*（写入 Frame.Err.Code）
//   - Go 侧用 Err*（*FrameError，实现 error，可用 errors.Is 按 Code 比较）
//   - 需要附带 message 时用 Errorf(Code, ...)

// constant.go 集中定义 Host 运行时用到的全部契约常量。
// 破坏线协议 / host 方法名 / hostFaces / 错误码时，先改这里再改实现。

// ---------------------------------------------------------------------------
// Frame 线协议（Host ↔ Plugin）
// ---------------------------------------------------------------------------

const (
	// FrameEvent / FrameRequest / FrameResponse 是 Frame.Type 的合法取值。
	FrameEvent    = "evt"
	FrameRequest  = "req"
	FrameResponse = "res"

	// FrameVersion 是当前 Host↔Plugin 线协议版本。
	// 新 host 包使用独立字段命名：version / capability / err（与旧 protocol 包不兼容）。
	FrameVersion = 1

	// FrameMaxSize 是单帧 JSON body 上限（16MB），超限拒绝读写。
	FrameMaxSize = 16 << 20
)

// ---------------------------------------------------------------------------
// Capability / Host 横切面
// ---------------------------------------------------------------------------

const (
	// HostCapability 是宿主横切方法的 capability 名（to=host 或 capability=host）。
	HostCapability = "host"

	// ToolsCapability 允许多属主（filetools/shelltools/webtools/skill-manager 等）。
	// Host 注册表对它不做唯一属主约束；路由仍按插件名点对点。
	ToolsCapability = "tools"
)

// Host L0 方法（to/capability == host 时分派）。领域方法不得出现在此列表。
const (
	HostMethodPlugins          = "plugins"
	HostMethodEnsurePlugins    = "ensurePlugins"
	HostMethodSetPluginEnabled = "setPluginEnabled"
	HostMethodPluginSwitch     = "pluginSwitch"
)

// hostFaces：插件声明后才允许 Medium/宿主按面调用（与 provides 语义不同，不得混用）。
const (
	HostFaceConfig   = "config"
	HostFaceCommands = "commands"
	HostFaceUI       = "ui"
)

// HostFaces 是合法 hostFaces 全集，供 manifest 校验与 CallByFace 门禁使用。
var HostFaces = []string{HostFaceConfig, HostFaceCommands, HostFaceUI}

// hostFace 方法名（Frame.Capability = face，Method 取下列值）。
const (
	// commands
	FaceMethodCall = "call"
	// ui
	FaceMethodAction = "action"
	// config（Settings / /refresh）
	FaceMethodReload = "reload"
	FaceMethodSchema = "schema"
	FaceMethodGet    = "get"
	FaceMethodSet    = "set"
)

// Frame.Err.Code 线格式错误码（稳定契约，插件/Medium 可依赖）。
const (
	// 路由 / 寻址
	CodeHostClosed       = "host_closed"
	CodeToRequired       = "to_required"
	CodeRouteFailed      = "route_failed"
	CodeMethodNotFound   = "method_not_found"
	CodePluginNotMounted = "plugin_not_mounted"
	CodePluginDown       = "plugin_down"
	CodePluginDisabled   = "plugin_disabled"
	CodeTimeout          = "timeout"
	CodeHandlerError     = "handler_error"
	CodeCallSelf         = "call_self"
	CodeHostServerClosed = "server_closed"
	// 载荷 / 参数
	CodeBadPayload    = "bad_payload"
	CodeBadArguments  = "bad_arguments"
	CodeFrameTooLarge = "frame_too_large"
	// Host 横切方法失败
	CodeEnsurePluginsFailed    = "ensure_plugins_failed"
	CodeSetPluginEnabledFailed = "set_plugin_enabled_failed"
	// 注册表
	CodeCapabilityConflict = "capability_conflict"
	// hostFaces 门禁
	CodeHostFaceNotDeclared = "host_face_not_declared"
	// Panel 结构校验（Host 只校验结构，不校验业务语义）
	CodePanelRejected = "panel_rejected"
)

// 哨兵错误（error）。比较请用 errors.Is(err, ErrXxx) 或 errors.Is(err, SomeFrameErr)。
// 需要 message 时不要改这些 var，用 Errorf(CodeXxx, "...")。
var (
	// 路由 / 寻址
	ErrHostClosed       = &FrameError{Code: CodeHostClosed}
	ErrToRequired       = &FrameError{Code: CodeToRequired}
	ErrRouteFailed      = &FrameError{Code: CodeRouteFailed}
	ErrMethodNotFound   = &FrameError{Code: CodeMethodNotFound}
	ErrPluginNotMounted = &FrameError{Code: CodePluginNotMounted}
	ErrPluginDown       = &FrameError{Code: CodePluginDown}
	ErrPluginDisabled   = &FrameError{Code: CodePluginDisabled}
	ErrTimeout          = &FrameError{Code: CodeTimeout}
	ErrHandlerError     = &FrameError{Code: CodeHandlerError}
	ErrCallSelf         = &FrameError{Code: CodeCallSelf}
	ErrHostServerClosed = &FrameError{Code: CodeHostServerClosed}
	// 载荷 / 参数
	ErrBadPayload    = &FrameError{Code: CodeBadPayload}
	ErrBadArguments  = &FrameError{Code: CodeBadArguments}
	ErrFrameTooLarge = &FrameError{Code: CodeFrameTooLarge}
	// Host 横切方法失败
	ErrEnsurePluginsFailed    = &FrameError{Code: CodeEnsurePluginsFailed}
	ErrSetPluginEnabledFailed = &FrameError{Code: CodeSetPluginEnabledFailed}
	// 注册表
	ErrCapabilityConflict = &FrameError{Code: CodeCapabilityConflict}
	// hostFaces 门禁
	ErrHostFaceNotDeclared = &FrameError{Code: CodeHostFaceNotDeclared}
	// Panel 结构校验
	ErrPanelRejected = &FrameError{Code: CodePanelRejected}
)

// ---------------------------------------------------------------------------
// 在途调用 wait
// ---------------------------------------------------------------------------

// WaitKind 区分一次 pending 的发起方。
type WaitKind int

const (
	// WaitHost 宿主主动调插件，结果走 channel 回调用方。
	WaitHost WaitKind = iota
	// WaitPlugin 插件 A 调插件 B，宿主仅中转并还原 frame id。
	WaitPlugin
)

// 在途调用 id 前缀（宿主改写后对插件透明；插件视角发/收 id 保持不变）。
const (
	// ForwardIDPrefix 插件→插件转发时宿主分配的内部 id（fwd-N）。
	ForwardIDPrefix = "fwd-"
	// HostCallIDPrefix 宿主主动 callOnce 时分配的 id（host-N）。
	HostCallIDPrefix = "host-"
)

// ---------------------------------------------------------------------------
// 生命周期 / 持久化
// ---------------------------------------------------------------------------

const (
	// DefaultCallTimeout manifest 未声明 timeout_ms 时的默认调用超时。
	DefaultCallTimeout = 30 * time.Second
	// DefaultShutdownGrace Close 时 stdin EOF 后等待子进程退出的宽限期，超时 killTree。
	DefaultShutdownGrace = 2 * time.Second
	// SwitchFileName 插件启用/禁用名单文件名（位于 pluginsDir 下）。
	SwitchFileName = ".plugin-switch.json"
)

// ---------------------------------------------------------------------------
// 呈现 / 事件中继（Host 不解释 payload，只按 cap/method/topic 分流）
// ---------------------------------------------------------------------------

// Presentation capability 与 method（无 id = 广播扇出）。
const (
	PresentationCapability   = "presentation"
	PresentationMethodCard   = "card"
	PresentationMethodRender = "render"
	PresentationMethodPanel  = "panel"
	PresentationMethodStream = "stream"
	PresentationMethodStatus = "status"
)

// 事件总线 topic（Host publish → Medium/SSE）。
const (
	TopicPresentation = "presentation"
	TopicStatus       = "status"
	TopicStream       = "stream"
	TopicPanel        = "panel"
	// TopicEvent 无 id 且非 presentation 的泛化插件事件（如 choice.ask）。
	TopicEvent = "evt"
)

// Presentation status 常用值（Medium turn 生命周期也复用）。
const (
	StatusIdle    = "idle"
	StatusRunning = "running"
)

// RenderIntent.kind（协议 v2 分类）。
const (
	RenderKindMarkdownText = "markdown_text"
	RenderKindMessageText  = "message_text"
	RenderKindSummaryText  = "summary_text"
)

// PanelOp.op 与 Shell 五区槽位（Host 结构校验）。
const (
	PanelOpSet   = "set"
	PanelOpClear = "clear"
)

const (
	SlotTop    = "top"
	SlotBottom = "bottom"
	SlotLeft   = "left"
	SlotCenter = "center"
	SlotRight  = "right"
)

// UISlots 是 PanelOp.Slot 合法值全集。
var UISlots = []string{SlotTop, SlotBottom, SlotLeft, SlotCenter, SlotRight}

// ComponentTagSeparator 是 Panel Component 标签中插件名前缀的分隔符。
// 约束：component 必须以 "<pluginName>-" 开头。
const ComponentTagSeparator = "-"

// StreamPayload.op（LLM 流式通道）。
const (
	StreamOpStart = "start"
	StreamOpChunk = "chunk"
	StreamOpEnd   = "end"
)

// Stream channel，区分正文与 reasoning。
const (
	StreamChannelContent   = "content"
	StreamChannelReasoning = "reasoning"
)

// 插件挂载状态（Plugin Graph / host.plugins 快照）。
const (
	PluginStateMounted   = "mounted"
	PluginStateAvailable = "available"
	PluginStateDegraded  = "degraded"
	PluginStateDisabled  = "disabled"
	PluginStateMissing   = "missing"
)

func Errorf(code string, format string, args ...interface{}) *FrameError {
	return &FrameError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}
