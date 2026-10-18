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

`Serve()` 先上报已注册方法，再读取 stdin 直到连接关闭。

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
```

注册方法：

```go
p.Register(
    "example",
    "run",
    pluginsdk.NewHandler(handleRun).WithInfo(
        "run",
        "Run an example operation",
    ),
)
```

路由键是 `capability.method`。同插件重复注册同键时，最后一次生效。

`From` 是 Host 注入的真实插件来源。它只用于身份和溯源，不参与路由。插件不能通过 payload 伪造该字段。

handler 返回：

- `payload == nil`：SDK 返回 JSON `null`；
- `error == nil`：返回 marshal 后的 payload；
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

Call 同步发起 Request，并等待同 id 的 Response。`ctx` 不能为 `nil`。

嵌套 Call 必须传递 Handler 收到的同一个 `ctx`。Context 取消会发送 `cancel` Frame，并等待原 Request 的唯一终态。普通取消返回 `call_cancelled`。超过期限返回 `call_deadline_exceeded`。

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

## 5. Emit

```go
Emit(capability, method string, payload json.RawMessage) error
EmitWithID(id, capability, method string, payload json.RawMessage) error
```

`Emit` 发送无 id 的广播 Event。当前 Host 会丢弃广播 Event。

`EmitWithID` 发送属于在途 Call 的归因 Event。id 必须指向仍存在的 pending 项。

## 6. 错误

```go
pluginsdk.ErrCode(code, message) error
pluginsdk.Code(err) string
```

无 code 的错误使用 `handler_error`。插件需要细分错误时，先定义稳定 code，再写入该插件文档。

## 7. WebUI Interface

目标 pluginsdk WebUI 面如下：

```go
func (u *WebUI) SetState(mountID string, state json.RawMessage) error
func (u *WebUI) SendCommand(mountID string, command string, payload json.RawMessage) error
```

目标语义：

- `mountID` 与当前插件名组成实例地址；
- `SetState` 表示可重建 State；
- `SendCommand` 表示临时 Command；
- 两者只发送到本插件拥有的 mount；
- 两者不需要额外权限声明；
- 不新增 `WebUI.Call`；
- 动态 Component 解析推迟到 sandbox 阶段。

当前 `Plugin.WebUI()` 返回 `WebUI`，上述接口已经实现。

`SetState` 内部调用 `host.ui.state`。`SendCommand` 内部调用 `host.ui.command`。Host 校验目标后将 Push 放入浏览器队列。

调用成功不表示浏览器已经接收。浏览器离线、队列已满和浏览器重启都不作为插件错误。

## 8. 类型边界

插件内部类型不得直接成为平台公共契约。插件需要：

1. 接收自己的方法 payload；
2. 转换为稳定 View 或 Command payload；
3. 丢弃传输层字段；
4. 对外只暴露必要业务字段。

插件 Event 和 State 的 JSON Schema 写在插件 `communication.md`。

## 9. 限制

当前 v1 不提供：

- 通用广播扇出；
- 插件自定义 Priority；
- 按插件名寻址；
- 异步 Call；
- 插件 shutdown Frame。

这些能力必须先更新协议，再修改 SDK。
