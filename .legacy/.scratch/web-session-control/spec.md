# Web：插件开关 · Session 权限档 · 聊天内确认

Status: resolved

## Problem

1. Host/Web 无法关闭插件；Autostart/ensure 会反复拉起。
2. Agent 权限停留在全局 sandbox config + 工作区 rules，无 Session 级三档。
3. Web 工具确认用 `window.confirm`；allow 路径常不落 Session Log。

## Decisions（用户定案）

| 项 | 定案 |
|----|------|
| 插件关 | 立即卸载 + 禁止再挂（Autostart/ensure/ensureAlive/launch 全过滤） |
| 核心插件 | 允许关闭，UI 警告（按 Manifest autostart，不写死目录名） |
| 默认 mode | `workspace_write` |
| mode vs rules | 模式是**默认档**；显式 allow 可放宽 |
| 中途改 mode | 允许，下一 tool call 生效 |
| 确认 UI | 聊天页确认卡，去掉浏览器弹窗；Host 契约不改 |
| 日志 | sandbox/agent 最终裁决都写 `policy_decision` |

ADR：0032（插件开关）、0033（Session Permission Mode）。

## Scope

- Web Medium UI；Host L0 开关面；session/agent/sandbox 契约。
- 不做：CLI 确认/开关 UI、OS sandbox、Host 扩大 agent.confirm、tools 名单门禁。

## Issues

- 01-host-plugin-switch
- 02-session-permission-mode
- 03-chat-approval-card
- 04-tests-and-docs
