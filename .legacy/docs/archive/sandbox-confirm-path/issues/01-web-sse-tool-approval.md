# 01 — events.js 订阅 tool_approval

**What to build:** `web/static/app/events.js` 的 EventSource 增加 `tool_approval` 监听并 `applySSE` fan-out，使 session-view 的 `LiteAgent.on('tool_approval')` 能弹确认。

**Status:** resolved

- [x] addEventListener tool_approval

## Answer

refactor-host-boundary Z1 对账确认：本票内容已在既有实现中落地（见上方勾选项
与对应 ADR/测试），状态由 ready-for-agent 滞后修正为 resolved，非本轮新开发。
