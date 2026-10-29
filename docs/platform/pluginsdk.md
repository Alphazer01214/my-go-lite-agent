# pluginsdk

`pluginsdk` 是插件进程的唯一开发入口。插件不手写 Frame 读写循环。

SDK 只定义平台调用面。插件业务 payload 由插件文档定义。

## 1. 生命周期

典型启动顺序：

```go
p := pluginsdk.NewPlugin("example-plugin")
p.Register("example", "run", handler)
return p.Serve()
```

`NewPlugin` 的插件名是进程身份。身份用于日志和 id 前缀，不参与业务路由。

`Serve()` 先在独立 goroutine 上启动接收循环，再上报此刻已注册的方法，然后阻塞到 stdin 关闭。顺序不能反：注册请求本身是一次 Call，必须已经有人在收 Response。

上报是一次快照：`Serve()` 之后再 `Register` 的 Handler 只在本进程内生效，不会再上报给 Host。需要新路由时由 Host 侧重新装载插件。

上报是尽力而为。上报失败不中断进程，`Listen` 继续运行。

标准注册只上报 `capability` 和 `method`。
Host 只把 `(capability, method)` 写进路由表。
`name` 和 `description` 是可选展示字段，不参与路由。

`Serve()` 返回 `Listen()` 的错误。stdin 关闭是正常收尾，宿主关闭插件进程时也走这条路径。

### 1.1 导出接口

| 符号 | 说明 |
|---|---|
| `NewPlugin(name string) *Plugin` | 绑定进程 stdio |
| `(*Plugin).Serve() error` | 上报注册并阻塞运行 |
| `(*Plugin).Listen() error` | 只跑接收循环，不上报 |
| `(*Plugin).Register(capability, method string, handler HandleFunc)` | 绑定路由键 |
| `(*Plugin).ListRegistered() []Registration` | 读注册快照 |
| `(*Plugin).Call(ctx, capability, method string, payload json.RawMessage)` | 发起同步 Call |
| `(*Plugin).CallInContext(ctx, contextID, capability, method string, payload json.RawMessage)` | 在执行上下文内发起 Call |
| `(*Plugin).CallWithCallback(ctx, capability, method string, payload json.RawMessage, callback func(*Event))` | 带归因 Event 的 Call |
| `(*Plugin).CallWithCallbackInContext(ctx, contextID, capability, method string, payload json.RawMessage, callback func(*Event))` | 两者组合 |
| `(*Plugin).Emit(capability, method string, payload json.RawMessage) error` | 发送广播 Event |
| `(*Plugin).EmitInContext(contextID, capability, method string, payload json.RawMessage) error` | 带上下文的广播 Event |
| `(*Plugin).EmitWithID(id, capability, method string, payload json.RawMessage) error` | 发送归因 Event |
| `(*Plugin).EmitWithIDInContext(contextID, id, capability, method string, payload json.RawMessage) error` | 带上下文的归因 Event |
| `(*Plugin).WebUI() *WebUI` | 取得本插件的 UI 发布器 |

## 2. 注册

```go
type Request struct {
    ID         string
    From       string
    ContextID  string
    Capability string
    Method     string
    Payload    json.RawMessage
}

type HandleFunc func(ctx context.Context, req *Request) (json.RawMessage, error)

type Handler struct {
    Name        string
    Description string
}

type Registration struct {
    Capability string
    Method     string
    Handler    Handler
}
```

注册方法：

```go
p.Register("example", "run", handleRun)
```

路由键是 `capability.method`。同插件重复注册同键时，最后一次生效。

`Register` 在以下情况 panic：

- `capability` 或 `method` 为空；
- `handler` 为 `nil`。

`handleFunc` 负责解析请求、调用业务函数和编码响应。
`Decode` 可把 `Request.Payload` 解码到业务结构体。

```go
func handleRun(
    ctx context.Context,
    req *pluginsdk.Request,
) (json.RawMessage, error) {
    var input RunRequest
    if err := pluginsdk.Decode(req, &input); err != nil {
        return nil, pluginsdk.ErrCode(
            pluginsdk.CodeBadArguments,
            err.Error(),
        )
    }

    result, err := run(ctx, &input)
    if err != nil {
        return nil, err
    }
    return json.Marshal(result)
}
```

`NewHandler`、`WithInfo` 和 `Handle` 保留为兼容符号。
它们不属于标准注册路径。
`WithInfo` 返回新副本，不修改原值。

`From` 是 Host 注入的真实插件来源。它只用于身份和溯源，不参与路由。插件不能通过 payload 伪造该字段。

handler 返回：

- `payload == nil`：SDK 不写 `payload` 字段。线格式上该键缺席；
- `error == nil`：SDK 原样发送 `json.RawMessage`；
- `error != nil`：返回稳定错误码和错误信息。

## 3. Call

```go
Call(
    ctx context.Context,
    capability string,
    method string,
    payload json.RawMessage,
) (json.RawMessage, error)
```

Call 同步发起 Request，并等待同 id 的 Response。`ctx` 不能为 `nil`，否则返回 `ErrContextRequired`。

嵌套 Call 必须传递 Handler 收到的同一个 `ctx`。Context 取消会尽力发送 `cancel` Frame。等待以调用方 Context 为准：普通取消返回 `call_cancelled`，超过期限返回 `call_deadline_exceeded`。迟到的 Response 会被丢弃，不参与超时判定。

`ctx` 在发起前已经结束时，Call 直接返回对应错误，不写 Frame。

Call 不指定目标插件。Host 按 `(capability, method)` 路由。`priority` 由 Host 管理，插件不能指定下一跳。

`CallInContext` 在 Go Context 之外再指定 Host Execution Context：

```go
CallInContext(
    ctx context.Context,
    contextID string,
    capability string,
    method string,
    payload json.RawMessage,
) (json.RawMessage, error)
```

Go Context 控制 Call 的等待和期限。`contextID` 只选择 Host 的 Plugin Instance。

`Request.Proceed` 继续 Frame Priority 流水线：

```go
result, err := req.Proceed(ctx, payload)
```

每个 Request 最多调用一次 `Proceed`。重复调用返回 `flow_already_proceeded`。`Proceed` 只把结果交给下一跳，不暴露 Host 内部路由。

`Proceed` 传入的 `payload` 为空时，Host 沿用原 Request 的 payload。

## 4. CallWithCallback

```go
CallWithCallback(
    ctx context.Context,
    capability string,
    method string,
    payload json.RawMessage,
    callback func(*Event),
) (json.RawMessage, error)
```

该方法在等待 Response 期间接收归因 Event。callback 必须快速且非阻塞。

Event 结构：

```go
type Event struct {
    ID         string
    ContextID  string
    Capability string
    Method     string
    Payload    json.RawMessage
}
```

Event 顺序与调用方接收顺序一致。当前 pending channel 容量是 32。慢 callback 可能造成 Event 丢弃。

callback 为 `nil` 时，Call 收到的归因 Event 全部丢弃。

## 5. Emit

```go
Emit(capability, method string, payload json.RawMessage) error
EmitWithID(id, capability, method string, payload json.RawMessage) error
```

`Emit` 发送无 id 的广播 Event。当前 Host 会丢弃广播 Event。

`EmitWithID` 发送属于在途 Call 的归因 Event。id 必须指向仍存在的 pending 项。

带执行上下文的变体是 `EmitInContext` 和 `EmitWithIDInContext`，额外接受 `contextID` 参数。

## 6. 错误

```go
pluginsdk.ErrCode(code, message) error
pluginsdk.Code(err) string
```

无 code 的错误使用 `handler_error`。插件需要细分错误时，先定义稳定 code，再写入该插件文档。

`Code` 会解开 `fmt.Errorf("%w", ErrCode(...))` 包装链。普通 `error` 返回空串。

SDK 重新导出了这些平台错误码，插件不必再 import `protocol`：

| 常量 | 值 |
|---|---|
| `CodeMethodNotFound` | `method_not_found` |
| `CodeHandlerError` | `handler_error` |
| `CodeBadPayload` | `bad_payload` |
| `CodeBadArguments` | `bad_arguments` |
| `CodeCallCancelled` | `call_cancelled` |
| `CodeCallDeadlineExceeded` | `call_deadline_exceeded` |
| `CodeFlowAlreadyProceeded` | `flow_already_proceeded` |
| `CodeServerClosed` | `server_closed` |
| `HostCapability` | `host` |

`ErrContextRequired` 表示传入了 `nil` 的 Go Context。

## 7. WebUI Interface

```go
func (u *WebUI) SetState(mountID string, state json.RawMessage) error
func (u *WebUI) SendCommand(mountID string, command string, payload json.RawMessage) error
```

语义：

- `mountID` 是插件内稳定的实例标识。Host 把它和发起方实例 id 组成完整地址；
- `SetState` 表示可重建 State；
- `SendCommand` 表示临时 Command；
- 两者只发送到本插件拥有的 mount；
- 两者不需要额外权限声明；
- 不新增 `WebUI.Call`；
- 动态 Component 解析推迟到后续版本。

`Plugin.WebUI()` 返回 `*WebUI`。

`SetState` 内部调用 `host.ui.state`。`SendCommand` 内部调用 `host.ui.command`。Host 校验目标后将 Push 放入浏览器队列。

调用成功只表示 Push 已入队，不表示浏览器已经渲染。没有浏览器连接、或队列已满时，Host 返回 `ui_not_mounted`，插件的 `SetState`/`SendCommand` 会拿到这个错误，可以重试或降级。

参数校验在 SDK 侧完成，返回 `bad_arguments`：

| 方法 | 校验 |
|---|---|
| `SetState` | `mountID` 非空；`state` 非空且不是 `null` |
| `SendCommand` | `mountID` 非空；`command` 非空；`payload` 为空时按 `{}` 发送 |

两个方法都用独立的 10 秒上限 Context 发起发布。发布不继承触发它的 Handler 的取消信号，因此 Handler 结束后发布仍然有效。

## 8. 运行时细节

### 8.1 接收循环

`Listen` 逐帧读 stdin 并分发：

| 帧类型 | 处理 |
|---|---|
| `req` | 记住 id，另起 goroutine 执行 handler |
| `res` | 交给对应的 pending |
| `evt` | 交给对应的 pending 回调 |
| `cancel` | 取消对应 Handler 的 Context |
| 其它 | 忽略，向前兼容 |

handler 在独立 goroutine 运行，慢 handler 不阻塞接收循环。每条 Request 恰好写一条 Response。

写入 stdout 使用独立的写锁。状态锁不覆盖管道 I/O，因此管道写阻塞不会卡住 pending 的完成。

### 8.2 取消先于请求

Host 转发 cancel 和 req 时不保证顺序。cancel 先到时，SDK 先登记一个占位项，等 req 到达时再取消该 Handler 的 Context。

被取消的 Request 最终返回的 Response 带 `call_cancelled` 或 `call_deadline_exceeded`，payload 为空。

占位项的窗口大小是 256 个最近的入站 id。超出窗口的占位项被淘汰。

### 8.3 连接断开

stdin 读失败时，SDK 关闭所有出站 pending 的 channel，并取消所有在途 Handler 的 Context。

出站 Call 此时返回一个没有错误码的普通 error，消息形如 `call <capability>.<method>: connection closed`。`pluginsdk.Code` 对它返回空串。

## 9. 类型边界

插件内部类型不得直接成为平台公共契约。插件需要：

1. 接收自己的方法 payload；
2. 转换为稳定 View 或 Command payload；
3. 丢弃传输层字段；
4. 对外只暴露必要业务字段。

插件 Event 和 State 的 JSON Schema 写在插件 `communication.md`。

## 10. 限制

当前 v1 不提供：

- 通用广播扇出；
- 插件自定义 Priority；
- 按插件名寻址；
- 异步 Call；
- 插件 shutdown Frame。

这些能力必须先更新协议，再修改 SDK。
