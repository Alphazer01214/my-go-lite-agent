# 03 — Windows VT + server 入口

**Answer:** `enableVirtualTerminal()` now runs in both `CLI()` and `Server()` so `liteagent-server -repl` also gets ANSI clear-line. Combined `-serve -repl` rebinds `OnToolApproval` to the CLI prompt (was left on the Web SSE waiter, which hung REPL turns).

**Status:** resolved
