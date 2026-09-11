# 12 — Plugin SDK（二次开发接口）

**What to build:** 仓库外开发者只实现 Handler 与 `plugin.json`，通过本仓库的 `pluginsdk` 启动合法插件进程；消费方用同一 SDK 经 Host 调用其他 Capability。echo 改为基于 SDK 的参考实现，证明作者路径可用。

**Blocked by:** 04 — Capability 星型路由

**Status:** ready-for-agent

- [ ] 存在可导入的 `pluginsdk`：`Serve` 主循环（stdin/stdout Frame、req 分发、退出即关管道）
- [ ] 支持 `Handle(cap, method, handler)` 注册；未注册 method 返回结构化 Frame 错误
- [ ] 支持消费方出站 `Call`（经 Host 路由，见 04）：超时与 `error` 字段映射为 Go error
- [ ] 支持发 `evt`（为 Presentation / 流式预留，本票不要求 UI）
- [ ] `plugins/echo` 改为基于 `pluginsdk` 实现，且既有 Host 集成测试仍绿
- [ ] 清单校验与 `protocol` 字段升版规则在包注释中写清（公开契约）
- [ ] 主缝测试：仅用 SDK 写的 echo 插件仍能通过 Host 回环；消费方 fixture 经 `Call` 打到 provider

## Answer

（待实现）

## Comments

- 06 及之后的 session / tool 等插件应基于 `pluginsdk`，不再手写 Frame 循环。
- 与 01–03 的关系：01 立协议；本票把协议收成**作者可依赖的 Go API**。
