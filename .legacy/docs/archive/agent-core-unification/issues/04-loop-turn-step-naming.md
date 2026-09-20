# 04 — Loop 命名与 Turn/Step 术语对齐

**What to build:** 代码命名与 CONTEXT.md 对齐：Turn/Step 成为正式词汇；消除 `round` 与 Step 的混用。

**Blocked by:** 02 — Turn/Step 边界事件

**Status:** resolved

- [x] `MaxToolRounds` 重命名为 `MaxSteps`，注释指向 CONTEXT.md Turn/Step
- [x] Loop 内部变量/注释统一用 Turn/Step，不再用 round 指代 step
- [x] `TurnResult` 等公开类型名保持稳定
- [x] 相关测试文案同步（错误信息改为 exceeded N steps）
- [x] 全量相关测试通过

## Answer

`serve.MaxToolRounds` → `serve.MaxSteps`；`RunTurn` 循环变量 `round` → `step`；错误文案 `exceeded %d model rounds` → `exceeded %d steps`。CLI 的 Frame round-trip 语义不变。

## Comments

- 纯命名票，不改行为。
