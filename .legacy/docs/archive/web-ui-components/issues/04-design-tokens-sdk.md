# 04 — Design Tokens 与 SDK 增量

**What to build:** Shell CSS 变量收敛为 `--la-bg / --la-panel / --la-ink / --la-dim / --la-line / --la-accent` 并作为组件公开契约（Shell 自身样式同源消费）。sdk.js：新增 `onSessionChange(fn)`——订阅即以当前 session id 回调一次，之后变更再触发；删除 `bind` 与 `data-la-*` 委托；`on / call / emitUIAction / complete` 不变。

**Blocked by:** —

**Status:** resolved

- [x] tokens 定义 + Shell 自身样式切换到 tokens
- [x] onSessionChange（订阅即回调语义）
- [x] 删 bind / data-la-*
- [x] sdk.js 头注释更新为组件契约摘要（插件作者文档入口）
- [x] TestShellAndSDKServed 适配

## Answer

`shell.html` 与 `trace.html` 的 `:root` 变量全部改名 `--la-*`（shell 另有 `--la-panel2/--la-err/--la-ok/--la-stop/--la-mono/--la-sans` 扩展 token；trace 自带同前缀子集），全部引用同步替换、旧名清零。`sdk.js` 重写：保留 `on/call/emitUIAction/complete`，`emitUIAction(plugin, panel, event, value, props)` 签名不变（plugin 必须显式传，模块从 `import.meta.url` query 自取，不能靠 `location`）；新增 `onSessionChange(fn)`（`window.__liteSessionId` 存在则订阅即回调；之后 Shell 经 `LiteAgent.emit('__session', id)` 触发）与公开 `emit(topic, data)`（Shell SSE 扇出用）；删除 `bind` 与 `data-la-*`。头注释改为组件契约摘要（含 URL/命名/Shadow DOM/tokens 约定）。`sdk/lite-agent.js`（仓库可拷贝版）与 `web/static/sdk.js` 同步为同一文件。

## Comments

- Shell 侧会话通道：`setSessionId(id)` 同步 `currentSessionId`、`window.__liteSessionId` 并 emit（值未变时不重复 emit，避免 refreshRunState 轮询噪声）。
- `on` 的 listeners 从固定 topic 表改为按需建表，配合公开 `emit`。
