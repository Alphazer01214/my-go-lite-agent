# 12 — Plugin SDK Serve（二次开发 · 服务端）

**What to build:** 仓库内外同一套作者接口：只实现 Handler 与 `plugin.json`，`pluginsdk.Serve` 即可成为合法插件。echo 改为 SDK 实现，证明原生路径与第三方路径没有分叉。

**Blocked by:** 03 — Assembly 挂载

**Status:** ready-for-agent

- [ ] 存在可导入的 `pluginsdk`：`Serve` 主循环（stdin/stdout Frame、req 分发、管道关闭即退出）
- [ ] 支持 `Handle(cap, method, handler)`；未注册 method 返回结构化 Frame 错误
- [ ] 支持发 `evt`（为 Presentation / 流式预留，本票不要求 UI）
- [ ] `plugins/echo` 改为基于 `pluginsdk`，既有 Host 集成测试仍绿
- [ ] `protocol` 字段与清单校验规则在包注释中写清（公开契约）
- [ ] 主缝测试：仅用 SDK 写的 echo 仍通过 Host 回环

## Answer

（待实现）

## Comments

- 本票只做 **Serve**；出站 `Call` 在 04（依赖星型路由）。
- 06 及之后所有原生插件必须基于 `pluginsdk`，不得手写 Frame 循环。
