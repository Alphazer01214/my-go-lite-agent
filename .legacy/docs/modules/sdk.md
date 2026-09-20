# sdk — Platform Module（lite-agent.js）

规范名词：[Platform Module](../../CONTEXT.md)、[UI Action](../../CONTEXT.md)、[Design Token](../../CONTEXT.md)。相关 ADR：0012、0030。

## 职责

Web Medium 暴露给 **Panel Component** 作者的 JS SDK 面。全局 `LiteAgent`。

只做 **L0**：点名 call、UI Action、事件总线、slash 补全。**领域 API（如 sendMessage）不在 SDK**，在各插件自己的 `ui/` 模块。

## 源与分发

| 路径 | 角色 |
|------|------|
| `sdk/lite-agent.js` | **作者源**（唯一真源） |
| `web/static/sdk.js` | 嵌入副本，由 `web/gen_sdk.go` 复制 |
| HTTP `GET /sdk/lite-agent.js` | Shell 与插件组件实际加载的资源 |

同步：`//go:generate`（在 `web/`）；`sdk_sync_test.go` 在副本漂移时失败。

## 公开 API（摘要）

| 方法 | 语义 |
|------|------|
| `LiteAgent.call(to, cap, method, payload)` | `POST /api/call` 点名转发 |
| `LiteAgent.callCap(...)` | 兼容/辅助（能力键由调用方给定） |
| `emitUIAction(plugin, detail)` | `POST /api/ui-action` |
| `on(event, fn)` / `emit(event, data)` | Shell 内事件总线（SSE 已 fan-out 到此） |
| `complete(prefix)` | Slash 补全辅助 |
| `runCommand(line)` | `POST /api/command` |

组件还依赖：

- Design Token（`--la-*`）
- `/plugin-ui/` 静态资源
- `/api/layout`、`/api/plugins`（装载器用）

## 事件

`events.js` 将 SSE topic 映射到 `LiteAgent.emit`：

- `presentation` / `status` / `stream` / `panel` / `evt`（`evt` = 无 id 插件 evt 通用中继，ADR-0034）

插件 Panel 订阅所需 topic；**不要**在 SDK 里实现会话/回合状态机。

## 与 Web Shell 的边界

| 属于 SDK | 不属于 SDK（插件 ui/） |
|----------|------------------------|
| call / ui-action / on / emit / command | Session View 业务 UI |
| 补全、总线 | 流式聊天拼装 |
| 无状态 | Scheme/Workspace 领域逻辑 |

## 开发约定

1. 保持 SDK 平台契约稳定；破坏性变更需同步 ADR 与出厂插件 UI。
2. 改源后必须重新 `go generate`，否则测试失败。
3. 不引入构建步骤依赖（原生 ES Module / 浏览器 API）。
4. 插件可依赖本 SDK，也可自带实现（平台契约可选依赖）。
