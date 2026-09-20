# Host 插件启用开关：Autostart 之上的减法覆盖

**Status: accepted**

## Context

ADR-0021 将日常挂载真源定为 Manifest `autostart` + `dependsOn` 闭包；Host 仅有幂等 `ensurePlugins`，无卸载/禁用面。Web 需要用户可关闭插件（含 autostart 核心件），且关闭后不得被 Autostart / Scheme ensure / 崩溃复活再次拉起。

Assembly 白名单已废弃，不能恢复为日常真源；开关也不能写进插件业务 config（Host 不解析领域 payload，ADR-0026/0023）。

## Decision

**Host 持有按插件名的 disabled 集合（L0 生命周期数据），对 Autostart 根集与 `ensurePlugins` / `ensureAlive` / `launch` 作减法过滤。**

- 存储：Host 自有 JSON（默认 `<pluginsDir>/.plugin-switch.json`，形如 `{"disabled":["name"]}`）。只存名字，不解析 Manifest 业务字段。
- API（L0，按插件名）：
  - `host.setPluginEnabled` `{name, enabled}` — 关：立即卸载进程/UI 并记入 disabled；开：清除 disabled（是否再挂载由后续 Autostart/ensure/Scheme 决定）。
  - `host.plugins` / Plugin Graph 附带 `disabled` 列表；节点状态新增 `disabled`。
  - `ensurePlugins` 对 disabled 名字返回 `disabled[]`，不 launch。
- 核心插件（`autostart=true`）**允许**关闭；UI 按 Manifest 字段警告，Host 不写死 `plugins/` 目录名（ADR-0030）。
- 被依赖插件关闭后，依赖方仍可挂载但 consumes 未满足 → `degraded`（ADR-0022，软失败）。
- 禁用生效路径必须同时覆盖：Start 过滤 Autostart 挂载集、EnsurePlugins、ensureAlive、launch。否则崩溃复活会绕过开关。

## Consequences

- 日常真源仍是 Autostart+dependsOn；开关是用户层减法，**不是** Assembly 恢复。
- 关闭 session/agent/llm 等会导致聊天金路径不可用——与 ADR-0017 软失败一致，进程仍起，用到能力时报错。
- Web Plugins Panel 可切换；Medium 经 `/api/call` `{to:"host", method:"setPluginEnabled"}` 寻址 Host L0 面。

## Relates

- ADR-0021 / 0023 / 0025 / 0030 / 0022 / 0017
