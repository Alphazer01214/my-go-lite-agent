# 08 — 工具路径

**What to build:** 默认 Loop 能把工具 schema 交给模型；fake-llm 发起 tool call 后，Host 经星型路由执行工具插件 Function，把 `tool/result` 落入 Session 并驱动下一步模型请求。

**Blocked by:** 07 — 最小一轮：默认 Loop + fake-llm

**Status:** resolved

- [x] 工具插件可注册面向模型的工具名与 schema，并由 Loop 送入模型请求
- [x] fake-llm 返回 tool call → Host 路由到工具插件 → 获得 result（或错误）
- [x] Session 中先有 tool 调用事实再有 tool 结果事实；下一步 Model Context 由日志派生
- [x] 工具 Function 不编排循环；是否继续由 Loop 决定
- [x] 主缝测试：一轮内至少「用户 → 模型调工具 → 工具结果 → 模型最终答复」
- [x] 工具插件与 fake-llm 基于 `pluginsdk` 实现

## Answer

工具面：Capability `tools`，方法 `list`/`call`（`plugins/echotool`）。默认 Loop（`serve.RunTurn`）启动时 `tools.list` 收集 schema 并随 `llm.complete` 送出；模型返回 `tool_calls` 时，先 `session.append(tool_call)`，再经星型 `tools.call`，再 `session.append(tool_result)`，然后用 `AgentRequest` 重建 Model Context 进入下一轮，直至无 tool_calls（上限 8 轮）。`session.derive` 投影 `message`/`tool_call`/`tool_result` 为 Model Context。`fakellm` 在有 tools 且尚无 tool 消息时返回一次 tool call，否则按 tool/user 内容作最终答复。主缝测试覆盖完整工具轮与无 tools 时的普通回复。

## Comments

- 工具 Function 只处理一次 call；是否继续由 Loop 决定。
- 错误路径：`tools.call` 失败会以 `error: …` 作为 tool_result 落日志，不中断 Loop（主缝测试 `TestToolCallErrorStillCompletesTurn`）。
- 多个插件同时 provide `tools` 会 fail-loud（Capability 唯一属主）；多工具应装在同一 Tools 插件内。
- Host `Message` 携带 `tool_calls`/`tool_call_id`，与 session derive 对齐；`messagesEqual` 比较 arguments。
- 一次模型响应的多个 tool_calls 合并为一条 `tool_call` 事实（meta.tool_calls 数组），再逐条 tool_result。
- 轮次上限：最多 `MaxToolRounds` 次模型请求；最后一次仍要求工具时直接报错，不再执行无法回喂的工具。
- Code review 修复：arguments 不变量、轮次边界、content+tool_calls 同条事实、错误路径测试。
