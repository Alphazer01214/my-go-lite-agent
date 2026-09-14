# 06 — Skills 注册占位

**What to build:** Context Manager 侧仅保留 skills **装载面**：`registerSkill` 或 `registerSegment`+kind，目录进入 System Prompt；不实现发现、不实现触发全文注入。

**Blocked by:** 02

**Status:** resolved

- [x] 注册 API（`context.registerSkill`）与目录段
- [x] 测试：注册后 assemble/prepare 含 skill 名
- [x] 触发注入明确留给后续 skills 发现票

## Answer

registerSkill 写入 skills 表并并入 assemble。

## Comments

- 对齐 Q9=B+C、Q23：发现与触发属后续 skills 插件票。
