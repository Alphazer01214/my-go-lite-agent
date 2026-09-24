# pluginsdk — 插件开发者接口

`pluginsdk` 是插件唯一入口。**只 import 本包**（测试夹具可用 `protocol`）；不手写 Frame 循环；只点名 `capability.method`，**不点名目标插件名**。线协议以 [protocol.md](protocol.md) 为准，名词以 [CONTEXT.md](../CONTEXT.md) 为准。

---

## 1. API 一览

| API | 职责 |
|-----|------|
| `NewPlugin(name) *Plugin` | 创建运行时（绑定 stdio） |
| `NewHandler(fn) Handler` / `WithInfo` / `Info` | 包装 handler，可选名称/描述 |
| `Register(cap, method, handler)` | 登记入站 `req` 的 handler |
| `Call(cap, method, payload)` | 同步外呼，等 `res`（归因 `evt` 丢弃） |
| `CallWithCallback(..., callback)` | 同步外呼，归因 `evt` 进 `callback` |
| `Emit` / `EmitWithID` | 发出无归属 / 归因 `evt` |
| `Serve() error` | `host.register` 上报后进入 `Listen` |
| `ListRegistered()` | 已注册键与 Handler 快照 |
| `ErrCode` / `Code` | 稳定错误码 |

```text
Call ──req──► Host ──► 对端 handler
   ◄───────── res ──────────┘          // 收到 res 后 pending channel close

CallWithCallback ──req──► …
   ◄── evt（0..n，callback）──
   ◄── res（1，返回值）──────

Listen：req → handler │ 归因 evt → 该次 Call │ res → 完成并 close
无 id evt → 丢弃；断连 → pending 全关，Call 返回 connection closed
```

---

## 2. 进程身份

```go
p := pluginsdk.NewPlugin("session")
```

- `name` 与目录名 / `plugin.json` 的 `name` 一致；仅用于日志、id 前缀（`session-1`），**不参与路由**。
- Frame 的 `from` 由 Host 注入，插件不填。

---

## 3. Register — 处理入站 req

```go
type Request struct {
    ID, Capability, Method string
    Payload json.RawMessage
}

p.Register("session", "query", pluginsdk.NewHandler(func(req *pluginsdk.Request) (json.RawMessage, error) {
    return resultJSON, nil
}).WithInfo("query", "查询会话")) // WithInfo / Info 可选，不参与路由
```

| 约定 | 行为 |
|------|------|
| 键 | `capability.method` |
| 同进程重复注册 | last-wins |
| 未登记的 `req` | 回 `method_not_found` |
| `err == nil` | 成功，返回值作 `payload` |
| `err != nil` | `error_code=Code(err)`（空则 `handler_error`） |

```go
return nil, pluginsdk.ErrCode("bad_arguments", "limit must be > 0")
// 调用方：if pluginsdk.Code(err) == "bad_arguments" { … }
```

`Register` **只接 `req`**；`evt` 不进 handler、不写 `res`。

---

## 4. Call / CallWithCallback — 外呼

```go
// 只要最终结果
payload, err := p.Call("llm", "complete", reqJSON)

// 过程也要（流式 delta / progress）
payload, err := p.CallWithCallback("llm", "complete", reqJSON,
    func(ev *pluginsdk.Event) {
        // 快、非阻塞；在此拼 delta / 实时渲染
        // ev.ID / Capability / Method / Payload
    })
```

| 约定 | |
|------|--|
| 闭环 | 以 **`res` 为准**；收到后 close channel |
| 结束时机 | 收到 `res`（或断连）才返回；此前的归因 `evt` 进 `callback` |
| 无 `callback` 的 `Call` | 归因 `evt` 丢弃，不打断闭环 |
| 寻址 | 只写 `capability.method`；Host 转发到属主 |
| id | 形如 `session-1`；对端看到的转发 id 由 Host 改写 |
| 失败 | `err` 可用 `Code(err)`；**业务失败不用 evt 表达** |
| 并发 | 可多 goroutine 同时 `Call` |

`callback` 在接收循环上调用，必须**快且非阻塞**；重活自行丢 goroutine。

典型流式消费（调用方）：

```go
var b strings.Builder
final, err := p.CallWithCallback("llm", "complete", body, func(ev *pluginsdk.Event) {
    if ev.Method == "chunk" {
        b.Write(ev.Payload) // 按约定解码 delta
    }
})
// settle：最终内容以 final / err 为准
```

---

## 5. Emit / EmitWithID — 发出 evt

`evt` **不要求应答**。**`Call` 问并等；`Emit` 说但不等。**

```go
// 归因：属于某次在途 Call（流式、进度、步骤）
_ = p.EmitWithID(req.ID, "llm", "chunk", deltaJSON)

// 无归属：状态、旁路事实
_ = p.Emit("llm", "status", statusJSON)
```

| | `EmitWithID` | `Emit` |
|--|--------------|--------|
| `id` | 入站 `Request.ID`（只作**归因**，不是寻址） | 空 |
| 场景 | handler 内推流式 / progress | 生命周期、配置重载、后台完成 |
| 消费 | 该次 Call 的 `callback` | Host 观察 / 以后的订阅 |

**不要**用 `Emit` 表达业务失败（走 `res`）；需要对端结果或认领动作时用 `Call`。

---

## 6. Serve / Listen

```go
func main() {
    p := pluginsdk.NewPlugin("session")
    p.Register("session", "query", handleQuery)
    if err := p.Serve(); err != nil {
        log.Fatal(err)
    }
}
```

1. 后台 best-effort `Call("host", "register", …)` 上报 `(capability, method)`；
2. `Listen` 阻塞至 stdin 关闭。

| 入站 `type` | 行为 |
|-------------|------|
| `req` | 起 goroutine 执行 handler，写回一条 `res` |
| `res` | 投递 pending 后 **close** channel |
| `evt` | 有 id 且命中 pending → 该 channel（`Call` 丢弃 / `CallWithCallback` 进 callback）；无 id 或未命中 → 丢弃 |
| 其它 | 忽略 |

stdin 关闭 / 读失败 → `fail()`：关闭全部 pending，阻塞中的 Call 返回 `connection closed`。v1 无调用超时。

---

## 7. 错误码

| code | 含义 |
|------|------|
| `method_not_found` | 无此 handler / Host 无此 method |
| `handler_error` | handler 返回无 code 的普通 error |
| `bad_payload` / `bad_arguments` | 载荷或参数问题 |
| `route_failed` / `plugin_not_mounted` / `plugin_down` | Host 路由或属主进程问题 |
| `server_closed` | 服务结束导致 pending 失败 |

业务码可自定义；Host 不解释。常量：`CodeMethodNotFound` 等，完整表见 [protocol.md](protocol.md)。

---

## 8. 与 Frame 的关系

- 线类型唯一定义在 `protocol`；插件作者正常不接触 Frame。
- 可见字段：`id` / `type` / `capability` / `method` / `payload` / `error_code` / `error_msg`。
- **无 `to`**；**`from` 不填**。
- 线编码：4 字节大端长度 + JSON，上限 16 MiB。

---

## 9. v1 非目标

- 无 ctx / 取消 / 调用超时。
- 不按插件名寻址（无 `EmitTo` / `CallTo`）。
- 无 id 的广播 `evt` 插件间扇出未实现（SDK 侧丢弃）。
- UI / hostFace 不在本包范围。
