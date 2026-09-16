# 02 — Context Manager：context.prepare / compact / usage / listContext

**What to build:** `plugins/context-manager` 增加 Capability `context`：

- `prepare`：入 sessionId + 由 Loop 传入的 derive messages（或等价）；出 `{messages, tools, systemText, usage, compactHint}`。CM consumes `tools` 以收集 schema；system 组装复用既有注册表。
- `compact`：入 messages（+sessionId）；出 `{summary, coversThroughSeq}` 建议值；**不**自行 session.append。
- `usage` / `listContext`：供 CLI 观测。
- `registerSkill`（或 segment kind=skill）：目录进 sys prompt；全文触发注入不做。
- 保留 `system-prompt.*` 与 `assemble`（ADR-0006 兼容）。
- 状态：全局 Segment 注册表 + `map[sessionId]` 薄指针；并发安全。

**Blocked by:** 01（compact 消费方依赖 derive 语义）

**Status:** resolved

- [x] `context` 方法面与 payload 契约定稿并实现
- [x] prepare 收集 tools.list（无 tools 插件则 tools 空）
- [x] compact 纯函数/插件内策略，返回摘要与 covers 建议
- [x] registerSkill 目录段
- [x] 主缝测试：register 段 → prepare 含 systemText；有 tools 时 prepare.tools 非空；compact 输出可被断言

## Answer

`plugins/context-manager` 实现 `context.prepare/compact/usage/listContext/registerSkill`；全局注册表 + per-session 薄状态；prepare 经 star-call `tools.list`。

## Comments

- 对齐 Q8=B、Q20、Q19b=A、Q23。
- 无 CM 提供方时 Host/Loop 必须仍可 turn（最小 session+llm 装配）。
