# 03 — agent 传 severity + soft-fail missing

**What to build:** agent 从 tools.list 收集 severity，callTool→policy.decide 带上；dependsPlugins 缺目录时 soft warn 不中断 Turn。

**Status:** ready-for-agent

- [x] tools.list severity map
- [x] policy.decide payload.severity
- [x] ensureSchemePlugins missing soft-fail
- [x] tool_calling dependsPlugins: sandbox
