# sandbox

沙箱 / 权限规则插件：提供 Policy Capability（ADR-0019 / ADR-0033）。Agent Loop 在执行工具前查询裁决。

## 提供

- Capability `policy`
  - `decide` `{tool, arguments, workspace, sessionId, severity, permissionMode}` → `{action, reason, severity, permissionMode, approved}`
  - 返回给 Agent 的 `action` 是**最终** `allow` / `deny`：规则或 severity/模式映射为 `ask` 时，本插件经 Host `agent.confirm` 走 Medium 确认后收成 allow|deny
- Capability `config`（Web Settings，兼容默认）
  - `schema` / `get` / `set` / `reload`
  - 可写字段：
    - `defaultAction`（`allow` | `ask` | `deny`）
    - `severityPolicy`：`{low|medium|high → allow|ask|deny}`（`full_access` 或无 Session 模式时的回退）

## Session Permission Mode（ADR-0033）

Session 元数据 `permissionMode`（`ask` | `workspace_write` | `full_access`，默认 `workspace_write`；旧值 `read_only` 兼容映射为 `ask`，ADR-0034）由 agent 传入 decide。

**裁决序：**

1. 显式 `rules`（deny > ask > allow）——**可放宽**模式
2. 无规则命中 → 模式档：

| mode | low | medium | high |
|------|-----|--------|------|
| `ask` | allow | ask | ask |
| `workspace_write` | allow | path∈Workspace→allow，否则 deny | ask |
| `full_access` | severityPolicy / defaultAction | 同左 | 同左 |

3. 兜底 `defaultAction`

最终 allow/deny 由 agent 写入 Session Log `policy_decision`（含 permissionMode）。

## 规则来源（后者覆盖前者，浅合并）

1. `<插件 exe 目录>/config/permissions.json`
2. `<Workspace>/.liteagent/permissions.json`（项目层优先）

规则字段：`{tool, path, action}`；可选 `severityPolicy`。

## Manifest

- 非 autostart；由 Agent Scheme `coding` 拉起
