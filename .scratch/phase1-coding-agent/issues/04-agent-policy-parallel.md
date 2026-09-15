# 04 — agent：接 policy、ask 往返、只读并行

**What to build:** `plugins/agent` Loop 策略：

- 每个 tool call 执行前：`policy.decide`（无 policy 提供方 → allow，保持最小装配可跑）。
- `deny`：不执行工具，tool_result 写入拒绝原因；Session Log 有决策事实。
- `ask`：经 Host `agent.request` 请求确认；用户 allow 后执行，deny 则同上；CLI/Web 由 Render Medium 承接（最小：CLI 同步确认；Web 可后续 UI，但契约先定）。
- `tools.list` 结果中的 `readOnly: true` 工具在同 Step 内可并行；写/shell（无 readOnly）串行。无 readOnly 字段时视为非只读。
- payload 注入 `workspace`（来自当前 Session）。
- Turn 开始时若用户文本含 `$skill`，调 skill-manager 展开（依赖 08；可先留接口空实现）。

**Blocked by:** 02（多 tools）、03（policy）；展开依赖 08

**Status:** ready-for-agent

- [x] decide/ask/deny 路径 + Log
- [x] CLI ask 金路径（可用 stub）
- [x] 只读并行 / 写串行
- [x] workspace 注入 tools.call

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- ADR-0019；Q14/Q18。Plan 不做工具门禁（Q17）。
