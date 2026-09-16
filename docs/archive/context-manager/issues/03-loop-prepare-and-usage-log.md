# 03 — 默认 Loop 接线 prepare / compact / usage 落盘

**What to build:** Host 默认 Loop：

- 每 Step：`AgentRequest` 重建后调用 `context.prepare`（若无 CM 则退回现行为）；`llm.complete` 使用 prepare 的 messages+tools。
- 预算：优先供应商 usage；缺失则字符估算；超阈值或 hint 时调 `context.compact` 并 **Loop** append Context Summary 事实。
- system：assemble 后 hash 未变则不 append；变了才 append；每 Turn 仍可刷新但 derive 只认最后一条（票 01）。
- llm.complete 响应增加 usage 时，写入非 derive 的 log 事实或 step/request meta。
- 逐步去掉 Host 内对 CollectToolSchemas/安全提示硬编码的双路径（可分 commit）。

**Blocked by:** 01, 02

**Status:** resolved

- [x] Loop 调用 prepare 并以此构造 llm payload
- [x] compact 触发（阈值 `ContextBudgetChars`）+ append summary
- [x] system hash 跳过
- [x] usage 优先供应商、回落估算
- [x] 无 CM 时行为兼容
- [x] 主缝测试：`TestLoopSkipsDuplicateSystemAppend`、`TestLoopAutoCompactsWhenOverBudget`

## Answer

Host `runTurn`：PrepareContext → 超预算 CompactContext → append `context_summary` 并重挂 system；`lastSystemAfterSummary` 跳过重复 system。

## Comments

- 对齐 Q12=A、Q13=A+B、Q16=暂留 Host、Q18=A+chars 回落、ADR-0013。
- Agent 模式解耦不在本票。
