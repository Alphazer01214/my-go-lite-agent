# 05 — 生命周期与恢复

**What to build:** 插件从挂载到退出的完整生命周期可观测、可恢复：启动期依赖不满足 fail-loud；调用超时；崩溃后 Capability 变 unhealthy 且进行中调用失败；下次调用 on-demand 重拉；Host 退出时优雅结束插件（Windows 语义）。

**Blocked by:** 04 — Capability 星型路由

**Status:** resolved

- [x] Assembly/启动期：`consumes` 在拓扑拉起后仍缺失 → 明确失败并退出（fail-loud）
- [x] 调用超过清单/默认 `timeoutMs` → Host 向调用方返回超时错误
- [x] 插件进程退出 → 其全部 Capability 标记 unhealthy；相关进行中调用立即失败
- [x] 再次调用 unhealthy 插件时 on-demand 重新拉起；不无限空转重启
- [x] Host 退出：关闭插件 stdin → grace → 强制结束；主缝测试验证无孤儿进程（Windows）

## Answer

`serve` 增加：启动后校验 consumes；`timeoutMs`（默认 30s）；generation 保护的 on-demand 重拉；`Call` 对 `plugin_down` 重试一次；`Close` stdin→2s grace→Kill。主缝测试：未满足 consumes、slow 超时、crashonce 重拉、Host 退出不悬挂。

## Comments

- 旧 readLoop 的 EOF 不得误标新一代进程（`gen` 字段）。
