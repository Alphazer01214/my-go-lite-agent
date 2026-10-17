package host

import (
	"errors"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// 错误一律以 error 表达：
//   - 线格式用 Code*（写入 Frame.ErrorCode）
//   - Go 侧用 Err*（error 哨兵，可用 errors.Is 比较）
//   - 需要附带 message 时用 fmt.Errorf

// Frame 线协议唯一定义在 protocol；此处别名方便 Host 内部使用。
type Frame = protocol.Frame

const (
	FrameEvent     = protocol.FrameEvent
	FrameRequest   = protocol.FrameRequest
	FrameResponse  = protocol.FrameResponse
	FrameVersion   = protocol.FrameVersion
	FrameMaxSize   = protocol.FrameMaxSize
	HostCapability = protocol.HostCapability
)

var (
	WriteFrame       = protocol.WriteFrame
	ReadFrame        = protocol.ReadFrame
	ErrFrameTooLarge = protocol.ErrFrameTooLarge
)

// Host L0 方法（capability == host 时分派）。领域方法不得出现在此列表。
const (
	HostMethodRegister        = "register"
	HostMethodPlugins         = "plugins"
	HostMethodUIState         = "ui.state"
	HostMethodUICommand       = "ui.command"
	HostMethodContextCreate   = "context.create"
	HostMethodContextBind     = "context.bind"
	HostMethodContextClose    = "context.close"
	HostMethodInstanceCreate  = "instance.create"
	HostMethodInstanceClose   = "instance.close"
	HostMethodInstanceRestart = "instance.restart"
)

// FromHost 宿主合成帧的 from（protocol §3.5）。
const FromHost = "host"

// Frame.Err.Code 线格式错误码（稳定契约；字符串值在 protocol 包）。
const (
	CodeHostClosed                 = protocol.CodeHostClosed
	CodeRouteFailed                = protocol.CodeRouteFailed
	CodeMethodNotFound             = protocol.CodeMethodNotFound
	CodePluginNotMounted           = protocol.CodePluginNotMounted
	CodePluginDown                 = protocol.CodePluginDown
	CodePluginDisabled             = protocol.CodePluginDisabled
	CodeTimeout                    = protocol.CodeTimeout
	CodeHandlerError               = protocol.CodeHandlerError
	CodeServerClosed               = protocol.CodeServerClosed
	CodeBadPayload                 = protocol.CodeBadPayload
	CodeBadArguments               = protocol.CodeBadArguments
	CodeFrameTooLarge              = protocol.CodeFrameTooLarge
	CodeCapabilityConflict         = protocol.CodeCapabilityConflict
	CodeInstanceNotFound           = protocol.CodeInstanceNotFound
	CodeInstanceExists             = protocol.CodeInstanceExists
	CodeInstanceClosed             = protocol.CodeInstanceClosed
	CodeContextNotFound            = protocol.CodeContextNotFound
	CodeContextClosed              = protocol.CodeContextClosed
	CodeContextOwnerMismatch       = protocol.CodeContextOwnerMismatch
	CodeContextParticipantMismatch = protocol.CodeContextParticipantMismatch
	CodeContextBindingInvalid      = protocol.CodeContextBindingInvalid
)

// 哨兵错误（error）。比较请用 errors.Is(err, ErrXxx)。
var (
	ErrHostClosed       = errors.New(CodeHostClosed)
	ErrRouteFailed      = errors.New(CodeRouteFailed)
	ErrMethodNotFound   = errors.New(CodeMethodNotFound)
	ErrPluginNotMounted = errors.New(CodePluginNotMounted)
	ErrPluginDown       = errors.New(CodePluginDown)
	ErrPluginDisabled   = errors.New(CodePluginDisabled)
	ErrTimeout          = errors.New(CodeTimeout)
	ErrHandlerError     = errors.New(CodeHandlerError)
	ErrServerClosed     = errors.New(CodeServerClosed)

	ErrBadPayload   = errors.New(CodeBadPayload)
	ErrBadArguments = errors.New(CodeBadArguments)

	ErrCapabilityConflict = errors.New(CodeCapabilityConflict)
)

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
	ForwardIDPrefix  = "fwd-"
	HostCallIDPrefix = "host-"
)

// DefaultShutdownGrace 是 stdin EOF 后等待子进程退出的宽限期。
const DefaultShutdownGrace = 2 * time.Second

// PluginState* 是 Host 插件生命周期快照状态。
const (
	PluginStateMounted   = "mounted"
	PluginStateAvailable = "available"
	PluginStateDegraded  = "degraded"
	PluginStateDisabled  = "disabled"
	PluginStateMissing   = "missing"
)
