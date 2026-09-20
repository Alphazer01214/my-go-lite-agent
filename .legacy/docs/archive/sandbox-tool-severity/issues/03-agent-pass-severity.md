# 03 — agent 传 severity + soft-fail missing

**What to build:** agent 从 tools.list 收集 severity，callTool→policy.decide 带上；dependsPlugins 缺目录时 soft warn 不中断 Turn。

**Status:** resolved

- [x] tools.list severity map
- [x] policy.decide payload.severity
- [x] ensureSchemePlugins missing soft-fail
- [x] tool_calling dependsPlugins: sandbox

## Answer

refactor-host-boundary Z1 对账确认：本票内容已在既有实现中落地（见上方勾选项
与对应 ADR/测试），状态由 ready-for-agent 滞后修正为 resolved，非本轮新开发。
