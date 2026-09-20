# protocol — Frame 线编解码

规范名词：[Frame](../../CONTEXT.md)。线协议总览另见 [docs/protocol.md](../protocol.md)。本文件聚焦 **包级开发**。

## 职责

定义 Host ↔ Plugin 的唯一线消息格式：`uint32` 大端长度前缀 + JSON body。只管编码，不管路由、不解析领域语义。

**红线**：Host 不得按 `Cap` 做业务分支；寻址键是 `To`（目标插件名），`Payload` 一律 `json.RawMessage`（ADR-0030）。

## 文件

| 文件 | 内容 |
|------|------|
| `frame.go` | 全部类型、常量、`WriteFrame` / `ReadFrame` |
| `frame_test.go` | roundtrip、EOF、零长度拒绝 |

## 版本（两条线，勿混）

| 层 | 常量 | 位置 | 语义 |
|----|------|------|------|
| Frame 线协议 | `protocol.Version = 5` | `protocol/frame.go` | 消息编码与字段语义 |
| Manifest 契约 | `plugin.CurrentProtocol = 6` | `plugin/manifest.go` | `plugin.json` 可声明什么 |

- v2：Presentation render kinds → `markdown_text \| message_text \| summary_text`
- v5：引入 `To` 字段，Host 按插件名寻址 + 不透明 payload（ADR-0030）

## 导出 API

### `Frame`

```go
type Frame struct {
    V       int             `json:"v"`
    ID      string          `json:"id"`
    Type    string          `json:"type"` // req | res | evt
    To      string          `json:"to,omitempty"` // 目标插件名（ADR-0030）
    Cap     string          `json:"cap"`          // 接收方插件内 dispatch
    Method  string          `json:"method"`
    Payload json.RawMessage `json:"payload,omitempty"`
    Error   *FrameError     `json:"error,omitempty"`
}
```

| 字段 | 说明 |
|------|------|
| `V` | 写入时填 `protocol.Version` |
| `ID` | req/res 对齐键；广播 evt 可为空 |
| `Type` | `TypeReq` / `TypeRes` / `TypeEvt` |
| `To` | 目标插件名；空表示未点名（Host 会拒绝转发） |
| `Cap`/`Method` | **接收方插件**的 handler 键，不是 Host 路由表 |
| `Payload` | 不透明 JSON |
| `Error` | 仅 res 携带 |

### `FrameError`

```go
type FrameError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

实现 `error`，格式 `"code: message"`；`Error()` 对 nil 安全。

### 编解码

- `WriteFrame(w io.Writer, f *Frame) error` — marshal → 尺寸校验 → 4B BE 头 + body
- `ReadFrame(r io.Reader) (*Frame, error)` — 读头 → 拒绝 `n==0` 或 `n>max` → 读 body → unmarshal
- `maxFrameSize = 16 << 20`（16MB，未导出）
- 空流上 `ReadFrame` 返回 EOF

### 常见 `error.code`（非穷尽）

| Code | 含义 | 主要来源 |
|------|------|----------|
| `plugin_down` | 目标进程已退出 | `markUnhealthy` / 转发回写 |
| `timeout` | 调用超时 | Host 主调 / 转发 |
| `to_required` | Frame 缺少 `to` | `routeRequest` |
| `capability_unavailable` | cap 无 owner（残留路径） | 旧路由 |
| `route_failed` | 挂载/写帧失败或自调 | `forwardTo` |
| `unknown_tool` | tools.call 名未知 | （插件侧） |
| `method_not_found` | 未注册 `cap.method` | pluginsdk / Host host 面 |
| `handler_error` | Handler 普通 error | pluginsdk.dispatch |
| `host_closed` | Host 正在关闭 | `routeRequest` |
| `bad_payload` | Host 横切面解析失败 | `handleAgentFromPlugin` 等 |

## 依赖

仅 stdlib。被 `pluginsdk`、`serve`、出厂插件、`internal/app` 引用。

## 测试

- `TestFrameRoundtrip` — 全字段 + payload 字节等价
- `TestReadFrameEOF` — 空流
- `TestReadFrameInvalidLength` — 零长度拒绝

无 oversize-write 单测（写侧靠 `maxFrameSize` 守卫）。

## 开发约定

1. 破坏线格式（字段增删语义、编码变更）必须 bump `protocol.Version`，并同步升级出厂插件。
2. 不要在本包引入路由、能力语义或 Host 特化。
3. `Cap`/`Method` 只服务接收方 dispatch；Host 分支条件禁止使用业务能力名。
