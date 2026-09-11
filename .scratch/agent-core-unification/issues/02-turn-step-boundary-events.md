# 02 — Turn / Step 边界事件

**What to build:** Session Log 增加 turn/step 边界事实：`turn/start`、`turn/end`、`step/start`、`step/end`。默认 Loop 在层级切换时 append；`session.derive` 不把它们投影进 Model Context。

**Blocked by:** —

**Status:** resolved

- [x] Fact type 约定边界事件：`turn_start` / `turn_end` / `step_start` / `step_end`（与既有 `tool_call` 下划线风格一致）
- [x] Loop：RunTurn 开始/结束落 turn 边界；每个模型请求+工具执行单元落 step 边界
- [x] `session.derive` 忽略边界 type（只投影 message/tool_call/tool_result/system）
- [x] `session.query` 可列出边界事实供审计
- [x] 主缝测试：一轮完整工具路径后 query 到 1 个 turn + ≥2 个 step 边界；derive 仍只含模型可见消息

## Answer

`RunTurn` 在默认 Loop 路径写入 `turn_start` / `step_start` / `step_end` / `turn_end`（`role=host`，meta 携带 turn/step/reason）。失败路径经 defer 以 `reason=error` 落 `turn_end`。`session.derive` 原本只投影 message/tool_call/tool_result，边界 type 自动忽略。主缝测试 `TestTurnStepBoundaryEvents`（session+fakellm+echotool）。

## Comments

- end reason：completed / error / tools（step_end）。
- 与 `MaxToolRounds` 的改名见票 04。
- 边界事件是 log-only，不是 surface——对齐 dsh 分类。
- turn 编号 v1 固定为 1；多 Turn 编号待 Session multi-session / 多轮 CLI。
