# 04 — CLI 上下文观测

**What to build:**

- Turn 结束打印一行 usage（tokens 或估算、消息条数）。
- 插件或原生命令列出最近 n 条 Model Context 投影 / prepare 视图（`listContext` 或 session.derive + 过滤）。

**Blocked by:** 02, 03（数据源）

**Status:** resolved

- [x] usage 摘要行
- [x] 列表命令（`-context-list N`）
- [x] CLI 测试：`TestCLIContextUsageAndList`

## Answer

CLI `-context-list` + turn 后 `context usage=…`。

## Comments

- 对齐 Q22=B：Web 界面另票；本票保证数据源与 CLI。
