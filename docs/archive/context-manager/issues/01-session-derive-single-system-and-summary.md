# 01 — Session derive：单 system 投影 + Context Summary

**What to build:** `session` 插件 derive 规则：model-visible `role=system` 只投影**最后一条**；支持 Context Summary 事实——仅投影 active summary 及其 `coversThroughSeq` 之后的原文。append-only 不变，不提供 rewrite/replace。

**Blocked by:** —

**Status:** resolved

- [x] derive：多条 system 时只输出最后一条进 messages（**原位保留**，不提前到头部）
- [x] Context Summary 事实形状：`type=context_summary`，meta.active / meta.coversThroughSeq
- [x] derive：active summary 取代 covers 范围内旧 model-visible 事实；其后原文照常
- [x] 非 active 的旧 summary 不进 derive
- [x] 主缝测试：`TestSessionDeriveSingleSystemPrompt`、`TestSessionDeriveActiveContextSummary`

## Answer

`plugins/session` derive：先找最新 active `context_summary` 得 coversThroughSeq；再投影其后事实；`role=system` 的 message 只保留最后一条且留在时序位置（删除更早的 system）。summary 以 system 角色插在最后一条 System Prompt 之后（无则置于头部）。query 仍返回全部事实。

## Comments

- 对齐 Q17=A+B、Q11=C、ADR-0013。
- hash 跳过 append 由写入方（Loop，票 03）负责。
- 首版曾把最后 system 提到头部，打乱 Additional Contexts 顺序；改为原位投影后回归通过。
