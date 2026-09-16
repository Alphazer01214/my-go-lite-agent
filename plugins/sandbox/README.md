# sandbox

沙箱 / 权限规则插件：提供 Policy Capability（ADR-0019）。Agent Loop 在执行工具前查询裁决。

## 提供

- Capability `policy`
  - `decide` `{tool, arguments, workspace, sessionId, severity}` → `{action, reason, severity}`
  - 返回给 Agent 的 `action` 是**最终** `allow` / `deny`：规则或 severity 映射为 `ask` 时，本插件经 Host `agent.confirm` 走 Medium 确认后收成 allow|deny
- Capability `config`（Web Settings）
  - `schema` / `get` / `set` / `reload`
  - 可写字段：
    - `defaultAction`（`allow` | `ask` | `deny`）
    - `severityPolicy`：`{low|medium|high → allow|ask|deny}`（无显式规则时按工具严重程度拦截）

## 裁决顺序

1. 显式 `rules`（deny > ask > allow）
2. **severityPolicy**（工具 schema 上插件作者声明的 `severity`）
3. `defaultAction`

`ask` 在本插件内完成 Medium 往返（`agent.confirm` → Host `OnToolApproval` → CLI/Web）。

## 规则来源（后者覆盖前者，浅合并）

1. `<插件 exe 目录>/config/permissions.json`
2. `<Workspace>/.liteagent/permissions.json`（项目层优先）

规则字段：`{tool, path, action}`；可选 `severityPolicy`。

## 默认 severity（工具作者声明）

| tool | severity | 默认映射 |
|------|----------|----------|
| read_file / grep / glob / load_skill / web_* | low | allow |
| write_file / edit_file | medium | ask |
| shell | high | ask |

## Manifest

- 非 autostart；由 Agent Scheme `coding` 拉起
