# 02 — session derive：Tool Result Stub

**What to build:** Session 插件 `config.json` 字段 `fullToolResults`（默认 8）。derive 投影时，最近 K 条 `tool_result` 全文，更早的替换为 stub 文案（含原 content 字符数与恢复提示）。`tool_call` 永不 stub。log 不改。

**Blocked by:** —

**Status:** resolved

- [x] 配置装载
- [x] derive stub
- [x] 主缝测试 `TestSessionDeriveToolResultStub`

## Answer

`stubOldToolResults` 在 derive 输出后应用；`config.get/set/schema/reload` 暴露 `fullToolResults`。

## Comments

- ADR-0015。
