# 05 — Backlog：自动 compact 与 token 对账

**What to build:** （后续）Loop 在 compactHint 后自动 compact + append summary；provider usage 精确对账。

**Status:** resolved

## Answer

自动 compact 已由 agent 实现并接线（`maybeAutoCompact` + `contextProbe` latch，plugins/agent/main.go:313,938-943），经裁决立 ADR-0028 承认为默认行为；本 issue 的自动 compact 部分就此关闭。

provider usage 精确对账仍未做，转出为本 issue 遗留的独立 backlog（token 现为字符估算，阈值 0.80 见 context-manager `softBudgetRatio`）。

## Comments

- 2026-09-16：与 ADR-0013/0015 的「不做自动压缩」冲突已由 ADR-0028 显式 supersede，两篇旧 ADR 已加标注。
