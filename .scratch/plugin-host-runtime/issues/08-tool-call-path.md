# 08 — 工具路径

**What to build:** 默认 Loop 能把工具 schema 交给模型；fake-llm 发起 tool call 后，Host 经星型路由执行工具插件 Function，把 `tool/result` 落入 Session 并驱动下一步模型请求。

**Blocked by:** 07 — 最小一轮：默认 Loop + fake-llm

**Status:** ready-for-agent

- [ ] 工具插件可注册面向模型的工具名与 schema，并由 Loop 送入模型请求
- [ ] fake-llm 返回 tool call → Host 路由到工具插件 → 获得 result（或错误）
- [ ] Session 中先有 tool 调用事实再有 tool 结果事实；下一步 Model Context 由日志派生
- [ ] 工具 Function 不编排循环；是否继续由 Loop 决定
- [ ] 主缝测试：一轮内至少「用户 → 模型调工具 → 工具结果 → 模型最终答复」
- [ ] 工具插件与 fake-llm 基于 `pluginsdk` 实现
