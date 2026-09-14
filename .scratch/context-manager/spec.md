# Context Manager 完善（prepare / compact / usage / 单 system 投影）

Status: ready-for-agent

## Problem Statement

v1 Context Manager 只提供 `system-prompt` 组装；历史选择、压缩、tools/skills 注入与用量观测均无。Host 默认 Loop 自行 CollectToolSchemas、拼 messages，与「一切皆插件、Session Log 为唯一真源」逐渐两张皮。多轮还会重复 append 相同 system 事实导致 Model Context 膨胀。需要把「模型可见内容」收口到 Context Manager，并用日志原生的 Context Summary 实现可重建压缩。

## Solution

- Context Manager 扩权：保留 `system-prompt`，新增 `context` Capability（prepare / compact / usage / listContext）。
- Loop 在 `llm.complete` 前调用 `context.prepare`，使用返回的 messages + tools。
- Compact：Loop 超预算或 Command 触发 → `context.compact` → Loop append Context Summary 事实。
- Session derive：只投影最后一条 system；active Context Summary + 其后原文；其余旧事实留 Trace。
- usage：优先 llm 供应商 usage，缺失字符估算；落 log 供 CLI/Web。
- Host 本票不外置 Loop；Agent chat/agent「模式」解耦为后续。
- Waterfall/Interceptor **整条拆除**（独立票，代码与术语一并删）。

术语以 `CONTEXT.md` 为准；约束以 `docs/adr/0002`、`0006`、`0013` 为准。

## User Stories

1. 作为 Loop，我想调用 `context.prepare` 一次拿到 messages 与 tools，以便不再在 Host 里理解 schema 与选窗细节。
2. 作为使用者，我想超预算时自动 compact，且摘要进入 Session Log，以便重放时模型上下文仍可重建。
3. 作为使用者，我想 CLI 显示 token 占用并用命令列出最近 n 条上下文投影。
4. 作为会话审计者，我想 Session Log 保留每次 system 组装痕迹，但 Model Context 只含最新 System Prompt。
5. 作为后续 skills 插件作者，我想注册 skill 说明段；触发注入留到技能发现票。

## Implementation Decisions

- 单插件 `context-manager`：provides `system-prompt` + `context`；注册表全局一份，session 仅薄指针。
- prepare 契约见票 02；Host 仍强制 AgentRequest 与 derive 相等（ADR-0002）。
- Context Summary 为 model-visible 新事实；meta.active / coversThroughSeq；禁止 rewrite。
- 无 tools 提供方时的系统提示语义并入 Segment/prepare，避免 Host 硬编码旁路（票 03 可分步）。
- skills：本 feature 仅 register + sys prompt 目录；全文触发注入 out of scope。

## Testing Decisions

- 主缝不变：真实 Host + fixture 插件；不断言 Host 内部函数序。
- 必测：单 system 投影；hash 跳过重复 append；compact 后 derive 仍等于 AgentRequest；prepare.tools 与 tools.list 一致；usage 落 log；无 context 提供方时 turn 不失败（兼容最小装配）。

## Out of Scope

- Agent 模式（chat/agent 工具集）与 Loop 外置。
- skills 发现/触发注入完整路径。
- Web 详情页/悬浮（只保证 usage 数据源；界面另票）。
- token 精确对账与多 provider 自适应估算法。

## Further Notes

- 建议顺序：01 session derive → 02 CM prepare/compact/usage → 03 Loop 接线与 usage 落盘 → 04 CLI → 05 waterfall 清场 → 06 skills 占位。
- agent-core-unification 中「压缩 out of scope」由本 feature 接管。
