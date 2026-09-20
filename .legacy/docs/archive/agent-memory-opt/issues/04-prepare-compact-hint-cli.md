# 04 — prepare compactHint + CLI usage

**What to build:** prepare 入参可选 `contextWindow`；估算 token > window×0.80 时返回 `compactHint{suggestCompact, estimatedTokens, contextWindow, threshold}`。CM `/usage` 与 `context.usage` 一并展示。Loop 不自动 compact。

**Status:** resolved

- [x] compactHint
- [x] CLI 展示
- [x] 主缝测试 `TestPrepareCompactHint`

## Answer

`buildCompactHint`（softBudgetRatio=0.80）；usage 命令打印 contextWindow/compactHint。

## Comments

- 自动 compact 仍 deferred（票 05）。
