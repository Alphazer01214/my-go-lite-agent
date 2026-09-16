# 01 — events.js 订阅 tool_approval

**What to build:** `web/static/app/events.js` 的 EventSource 增加 `tool_approval` 监听并 `applySSE` fan-out，使 session-view 的 `LiteAgent.on('tool_approval')` 能弹确认。

**Status:** ready-for-agent

- [x] addEventListener tool_approval
