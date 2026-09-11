# 09 — 双层 Waterfall

**What to build:** Host 内建强制 Waterfall 链（至少审计与取消相关最低集）始终生效；外部 Interceptor 插件可挂载，对调用放行、改写或短路，且不能移除内建强制链。

**Blocked by:** 05 — 生命周期与恢复

**Status:** resolved

- [x] 内建强制链在每次相关调用上可观察（例如审计记录或测试探针）
- [x] Interceptor 插件经 Host 挂入 Waterfall，可改写 payload 或返回拒绝
- [x] 短路时下游不执行，调用方收到与拒绝语义一致的错误/结果
- [x] 卸载或崩溃 Interceptor 不导致强制链消失
- [x] 主缝测试：无 Interceptor / 允许 / 改写 / 拒绝 四种路径的外部行为

## Answer

`serve/waterfall.go`：每次星型 `routeRequest` 先跑双层链——内建 cancel（Host closed → reject）→ 可选外部 Interceptor（Capability `interceptor`，方法 `before`）→ 内建 audit（内存轨迹，`Server.Audit()`）。决策 `allow` / `rewrite`（改 payload）/ `reject`（`interceptor_rejected` 短路，下游不执行）。Interceptor 崩溃或 `plugin_down` 时 fail-open（`interceptor_down`），强制链与路由仍在。CLI `-audit` 打印 `audit from=… cap=… action=…`。Fixture：`plugins/interceptor`（mode.txt 控制 allow/rewrite/reject）、`plugins/crashix`。主缝测试覆盖无拦截、允许、改写、拒绝、崩溃后强制链仍在。

## Comments

- Waterfall 包住所有插件发起的星型调用（含 `agent/request`）；Host 内部 `Call`（Loop→session/llm/tools）本票不经过 Interceptor，避免自锁与递归。
- 多个 Interceptor 插件会因 `provides` 唯一属主 fail-loud；策略组合留给后续。
- 取消面目前是 Host closed；更细粒度 cancel token 留给后续票。
- Interceptor 失败默认 fail-open；策略面若需 fail-closed，应另立配置。
- 拒绝错误码：`interceptor_rejected` / `interceptor_error` / `interceptor_unknown_action` / `host_closed`。
- 空 payload 的 rewrite 被忽略（不擦除原 Call Payload）。
- Code review 修复：agent/request 也过链；错误码区分；空 rewrite；新增 agent/request 审计主缝测试。
- 「卸载」路径本票无现成卸载机制，崩溃覆盖已证明强制链不依赖 Interceptor 存活。
