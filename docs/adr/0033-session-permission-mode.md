# Session Permission Mode：会话级代理权限档

**Status: accepted**

## Context

Permission 裁决由 sandbox（provides `policy`）按规则 + severityPolicy 实现（ADR-0019）；规则源为进程级 `permissions.json` 与 Workspace 层覆盖。用户需要每个 Session 独立的粗粒度权限档：`read_only` / `workspace_write` / `full_access`，而不是只有全局 defaultAction。

ADR-0020 已将 Workspace 定为 Session 元数据。ADR-0023 明确 Agent Scheme **无** readOnly 工具名单门禁——门禁不应落在 `allowedTools`。

## Decision

**`permissionMode` 是 Session 元数据（与 Workspace 同构），由 policy 裁决消费；不是工具名单门禁，不恢复 Interceptor。**

取值：`read_only` | `workspace_write` | `full_access`。

### 裁决序（模式是默认档）

1. **显式 `permissions.json` rules**（deny > ask > allow）——可**放宽**模式（用户定案：显式 allow 覆盖模式）。
2. 无规则命中 → **Session `permissionMode` 档**：
   - `read_only`：severity low / readOnly 类 → allow；medium/high → deny。
   - `workspace_write`：low → allow；medium 且 path ∈ Session Workspace → allow，否则 deny；high → ask。
   - `full_access`：沿用 severityPolicy / defaultAction（现行为）。
3. 兜底：进程级 defaultAction。

### 契约

| 层 | 内容 |
|----|------|
| session | `SessionMeta.permissionMode`；`create` 可带；`setPermissionMode` 可改；`info`/`list` 返回；变更写 `session_meta` 事实。空值 → `workspace_write`。 |
| agent | `policy.decide` payload 增加 `permissionMode`（读自 session.info）；Subagent 默认继承父 Session。 |
| sandbox | decide 消费 `permissionMode`；ask 仍经 Host `agent.confirm`（Deferred，不扩大）；**最终 allow/deny 均应写入 Session Log `policy_decision`**。 |
| Web | New-session Face 三档 chips；Session 视图可随时切换。 |

### 明确不做

- 不改 Scheme `allowedTools` 做读写门禁（不与 ADR-0023 冲突）。
- 不把 permissionMode 放进 Host。
- 不新增浏览器弹窗；确认走 Session View 聊天页（Medium 展示变更）。

## Consequences

- 全局 sandbox `defaultAction` 降为兼容默认，不再是 Web 主控制面。
- 模式档 + 显式规则放宽：同一项目不同 Session 可有不同风险面。
- `policy_decision` 补齐 allow 路径后，确认结果可从 Session Log 重建（对齐「日志是真源」）。

## Relates

- ADR-0019 / 0020 / 0023 / 0029 / 0030
