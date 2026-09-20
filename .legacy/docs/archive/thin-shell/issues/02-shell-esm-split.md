# 02 — Shell 前端模块化拆分（零构建）

**What to build:** Web Medium 的内联单体页面脚本拆为多文件原生 ES modules：槽位布局、Panel 装载器、聊天面渲染、trace 面渲染、SDK 桥各自成模块；静态服务与 go:embed 方式不变，行为零变化。本票是后续「内容搬进插件」各票的 prefactor——先让改动容易，再实施改动。

**Blocked by:** —

**Status:** resolved

- [x] 静态资产为多文件 ES modules，不引入任何构建工具链
- [x] / 与 /trace 行为与拆分前一致（聊天、trace、插件面板、会话栏）
- [x] KaTeX 等 CDN 依赖不受影响；Design Token 契约不变

## Answer

815 行 shell.html → 172 行（CSS + DOM + 模块引用）；645 行内联 IIFE 逐行搬运为 9 个原生 ES modules：`state`（会话/运行状态与 setSessionId 桥）、`md`（markdown/数学渲染）、`facts`（reasoning 合并投影，chat 与 trace 共用）、`chat`（聊天面 + 流式 + Presentation 消费）、`trace`（trace 列 + dump）、`panels`（ADR-0010 装载器）、`rail`（会话栏渲染）、`events`（SSE → LiteAgent 扇出的 SDK 桥）、`main`（composer 与编排）；trace.html（92→39 行）的内联脚本 → `trace-page.js`。所有模块懒取 DOM、依赖无环；导出边界即 05/06/07 抽取接缝。

服务侧：`web/embed.go` 增 `//go:embed static/app`；server.go 增 `/app/` 路由（`fs.Sub` + FileServer + `Cache-Control: no-cache`——模块嵌在二进制内，缓存陈旧会跨 host 升级存活）。

验证：node --check 十个模块语法全过（仓库无 node 工具链，属本地校验）；web 包测试断言改为模块契约（shell 引用 /app/main.js、/app/events.js 含 EventSource）；全量 `-count=1` 绿。浏览器内真实执行模块链无自动化覆盖（与改动前内联脚本同等水平），由 05 起的插件化票逐步承接。
