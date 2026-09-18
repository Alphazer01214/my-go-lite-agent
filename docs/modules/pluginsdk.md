# pluginsdk — 插件作者 API 与共享契约

规范名词：[Frame](../../CONTEXT.md)、[Capability](../../CONTEXT.md)、[Presentation](../../CONTEXT.md)、[PanelOp](../../CONTEXT.md)、[UI Action](../../CONTEXT.md)、[Call Payload](../../CONTEXT.md)。

## 职责

两件事合一：

1. **插件进程内运行时**：`New` / `Handle` / `Call` / `CallTo` / `Emit` / `Serve`（stdio Frame）。
2. **共享契约包**：Presentation 线类型、PanelOp、RenderIntent、领域 Capability 名、模型侧 `Message`/`ToolCall`/`TurnResult`。Host 与插件都可 import，避免 Host 写死插件目录名。

**包注释**：生产代码不要手写 Frame 循环；破坏性变更需 bump protocol。

## 文件

| 文件 | 内容 |
|------|------|
| `server.go` | Server、Handler、Call/Emit/Serve |
| `presentation.go` | Presentation/commands/ui 常量、PanelOp、Card、RenderIntent、StreamPayload、领域 cap 名 |
| `types.go` | 模型侧 Message / ToolCall / TurnResult（Medium 仅可展示用） |
| `server_test.go` | handler 键映射 |

## Server API

| 方法 | 语义 |
|------|------|
| `New()` | 绑定进程 stdin/stdout |
| `Handle(cap, method, h)` | 注册；同键 last-wins |
| `Call(cap, method, payload)` | 星型调用（空 `To`）；**优先 `CallTo`** |
| `CallTo(plugin, cap, method, payload)` | 点名插件；空名拒绝 |
| `Emit` / `EmitTo(id, …)` | 广播 evt / 归属 in-flight 的 evt |
| `Serve()` | 读循环直到 EOF；req 开 goroutine 分发；res 完成 pending |

### 分发与错误

- 键：`cap + "." + method`
- 未注册 → `method_not_found`
- Handler 返回 `*FrameError` 透传；其他 error → `handler_error`
- stdout 写全部持锁；Call 写失败清理 pending
- Serve 读错误：`failPending` 后返回 `nil`（干净关闭）

## Presentation 契约

| 类型/常量 | 说明 |
|-----------|------|
| `RenderKind` | `markdown_text` \| `message_text` \| `summary_text` |
| `RenderIntent` | `Kind/Text/Level/Title/Pairs/Detail/SessionID`；**SessionID 禁止 omitempty**（空 = default） |
| `SummaryPair` | `Key/Value` |
| `StreamPayload` | `Op`（start\|chunk\|end）、`Delta`、`Channel`（content\|reasoning）、`SessionID` |
| `PanelOp` | `Op`（set\|clear）、`Slot`、`ID`、`Component`、`Props` |
| `Card` | `CardType`、`Tool`、`Data` |

便捷方法：`EmitPanel`、`EmitCard`、`EmitRender`、`EmitMarkdownText`、`EmitMessageText`、`EmitSummaryText`、`EmitStream`、`EmitStreamTo`、`EmitStatus`。

hostFace 相关常量：`PresentationCap`、`CommandsCap`、`UICap` 及各 method/evt 名。

## 领域 Capability 名（契约常量，非目录名）

`SessionCap`、`AgentCap`、`LLMCap`、`SystemPromptCap`、`ContextCap`、`LoopCap`、`ToolsCap`、`ProbeCap`。

这些是**共享契约字符串**，不是 Host 调度键。Host 路由只认插件名（`To`）。`ProbeCap`/`ProbeMethod` 用于 echo 探测，**禁止**当作插件目录引用。

## 模型侧类型（`types.go`）

`Message`、`ToolCall`、`TurnResult`（`User`/`Assistant`/`Chunks`/`ToolCalls`/`Messages`）。

- Loop/Session 插件用它们表达模型面契约
- Host **不得**编排或解析这些类型做业务
- Medium 允许 import 做展示（ADR-0030）

## 依赖

`protocol` + stdlib。被全部出厂插件、`serve`、`internal/app`、部分测试引用。

## 开发约定

1. 插件作者只经 pluginsdk 说话；夹具可手写 Frame。
2. 已知目标插件名时用 `CallTo`；`Call` 为兼容星型遗留。
3. 破坏本包公开类型语义 → 对应 bump protocol（Frame 或 Manifest）。
4. 流式状态机归插件 Panel / CLI turnRenderer；Shell 不拼聊天流。
