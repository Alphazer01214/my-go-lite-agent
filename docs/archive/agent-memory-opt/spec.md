# Agent 记忆优化（filetools 上限 / derive stub / contextWindow 观测）

Status: in-progress

## Problem Statement

长会话中 Model Context 被历史 tool 结果（尤其大文件读）撑爆；`context.prepare` 原样回传，无预算观测。Session Log 不变量要求缩窗只能经投影（ADR-0002/0013），不能 prepare 旁路改写。

## Solution

- filetools：`read_file` 默认 limit=500（硬顶仍 5000），截断提示 offset/limit。
- session derive：`fullToolResults`（默认 8）之外的历史 `tool_result` → Tool Result Stub；log 全量。
- llm-openai：`llm.info` 暴露 `contextWindow`（config/env 可覆盖）。
- context-manager prepare：接收 `contextWindow`，超 0.80 时返回 `compactHint`；Loop **不**自动 compact。
- CLI `/usage` 展示 estimatedTokens / window / suggestCompact。

## Out of Scope

- 自动 `context.compact` / Loop 写 summary
- provider token 精确对账
- Web Settings UI（契约见 `.scratch/plugin-config`）

## Seams

主缝：CLI 集成（append/derive/prepare/llm.info）；filetools 可单测 dispatchTool。
