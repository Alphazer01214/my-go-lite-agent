# Shell 缩薄（thin-shell）

Status: ready-for-agent

## Problem Statement

Web UI 内嵌于单一 `host.exe`：Shell 内建聊天主流程与 trace 页，槽位词汇、事件桥、聊天渲染都是 Host 的一部分。插件虽可经 Panel Component 注入（ADR-0010），但内容面不可替换、前端是 769 行单文件。需要「前后端分离」在插件层面的兑现：项目至多提供整体 layout、整体样式表、必要全局脚本，其余由插件作者实现组件并注册进页面。

## Solution（决策与理由见 docs/adr/0011）

1. **双入口**：liteagent-cli.exe（内核 + CLI Medium）与 liteagent-server.exe（内核 + Web Medium，保留 `-repl` 组合），共享同一内核与插件协议，分歧仅在前端；Web Medium 内建于 server 进程，protocol 保持 2。
2. **Shell 缩薄**：layout（页面 + Panel 槽位）、Design Token 样式表、SDK 与通用装载器；聊天主流程移出 Shell（ADR-0009/0010 的「插件不得替换聊天主流程」废止）。
3. **内容面 = session 插件的 Web 面**：session-view / session-rail / session-trace 三个 Panel Component；事实经 capability 调用自取（`call('session','query')`）；/api/history、/api/trace 退役；「当前会话」暂留媒介层。
4. **插件侧契约**：`ui/` 多文件资产包（html/css/js 各归其位）；UI-only 插件合法化（`entry` 可省）；不采纳 `have_web_ui` 布尔——声明是 `ui` 块（作者事实），挂载归 Assembly（用户意图）。

## Tickets（依赖序）

| # | 票 | Blocked by |
|---|-----|------------|
| 01 | 双入口拆分 | — |
| 02 | Shell 前端模块化拆分 | — |
| 03 | UI-only 插件合法化 | — |
| 04 | 多文件 ui 资产契约 + uidemo 迁移 | 03 |
| 05 | trace 视图插件化 + layout 页面机制 | 02, 04 |
| 06 | 会话栏插件化 | 05 |
| 07 | 主会话视图插件化（渲染） | 05 |
| 08 | 输入框迁移，Shell 归零内容 | 07 |
| 09 | /api/history 退役 | 07 |

## 后置（不在本期，触发条件见 ADR-0011）

「当前会话」下沉为 Capability；Web Medium 升级为插件进程（需事件订阅设施）；前端构建链（esbuild/vite）；Assembly 级 web UI 开关；不可信插件的 iframe 岛。

术语以 `CONTEXT.md` 为准（Session View / Trace / Shell / Panel 新义）；约束以 ADR-0011 为准。
