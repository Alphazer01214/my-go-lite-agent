# Web UI 契约 v2（web-ui-contract-v2）

Status: resolved (ticket 09 Playwright golden path remains open)

## Problem Statement

Web 开箱完全无法对话：SDK 注释声称 `sendMessage`/`runCommand` 但未实现；聊天槽高度链断裂导致内容裁切、composer 够不着。根因是 ADR-0011 契约残缺——slot/page 钉死 Host、Assembly 无 UI 控制权、平台模块非正式、SDK 双拷贝漂移。

## Solution（决策见 docs/adr/0012）

磁盘 Layout 真源 + 插件加法贡献 + Assembly 裁决 + protocol 3 + 当前会话下沉 session Capability + 平台模块入作者 SDK + 离线零 CDN。

## Seams（TDD 预先约定）

1. `plugin.Manifest.Validate` — ui.slots/pages、trust 字段
2. `layout.Load` / `layout.Merge` — 磁盘 layout 与贡献合并
3. `assembly` UI 覆盖解析 — 禁用/改 props/slot/胜者
4. HTTP 面 — `/sdk/lite-agent.js` 含 sendMessage；`/api/layout` 合并结果；session current 走 `/api/call`
5. 浏览器黄金路径（Playwright，dev-only）

## Tickets

| # | 票 | Blocked by |
|---|-----|------------|
| 01 | 修致命路径：SDK 方法 + 槽高度链 | — |
| 02 | Layout 包 + 磁盘加载 + -layout | — |
| 03 | Manifest ui.slots/pages/trust + protocol 3 | — |
| 04 | Assembly UI 裁决 | 02, 03 |
| 05 | Shell 按合并 layout 导航与挂载 | 02 |
| 06 | 当前会话下沉 session Capability | — |
| 07 | SDK 单源双出口 + 平台模块 + 去 CDN | 01 |
| 08 | session/uidemo 迁移 + examples/layout 发行 | 03–07 |
| 09 | Playwright 黄金路径 + 全量回归 | 08 |
