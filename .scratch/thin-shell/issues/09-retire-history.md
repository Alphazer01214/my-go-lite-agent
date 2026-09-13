# 09 — /api/history 退役

**What to build:** 最后一个消费者（session-view）已改走 capability 调用，删除 /api/history 专用端点及其遗留消费代码，回归确认无消费者。此后 Web Medium 对 session 的依赖只剩通用协议：capability 调用 + SSE + 会话选择的媒介接口。

**Blocked by:** 07

**Status:** ready-for-agent

- [ ] /api/history 端点移除，全部功能回归通过
- [ ] Web Medium 不再存在 session 专用只读端点（会话选择媒介接口除外，属后置项）
