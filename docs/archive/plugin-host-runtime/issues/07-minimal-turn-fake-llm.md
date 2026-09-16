# 07 — 最小一轮：默认 Loop + fake-llm

**What to build:** 用户提交一条输入后，默认 Agent Loop（在 Host 内）驱动完整聊天轮次：写 Session → 组装 Model Context → fake-llm 流式返回 → assistant 结果落 Session。本票不含工具。

**Blocked by:** 06 — Session 插件与日志不变量

**Status:** resolved

- [x] 默认 Loop 使用 Host 内建实现，注入 session/llm（及 loop 所需最小 Capability）
- [x] 用户输入成为 Session 事实，并出现在下一次 Model Context
- [x] fake-llm 插件可流式 `evt` chunk，最终 `res` 形成 assistant 消息并追加进日志
- [x] 一轮结束后 Host 可干净 idle 或退出（按入口约定）
- [x] 主缝测试：单条用户消息 → 可派生出完整 assistant 回复；全程不依赖真实网络 LLM

## Answer

Host `serve.RunTurn` 实现默认 Loop（ADR-0003）：`session.append(user)` → `AgentRequest(nil)` 强制日志不变量 → `CallStream(llm.complete)` 收集流式 `evt` chunk → `session.append(assistant)` → `derive` 返回完整 Model Context。`pluginsdk` 新增 `EmitTo`；`serve.CallStream` 把带 id 的 `evt` 归到对应 Call。`plugins/fakellm` 提供 `llm.complete`，流 3 个 chunk 后返回 `You said: …`。CLI：`-turn <input>`，可与 `-session-derive` 组合。主缝测试：一轮成功（chunk + assistant 可派生）；无 llm 提供方时 fail-loud。

## Comments

- Loop 在 Host 进程内，经同一 `Call`/`AgentRequest` 路径；若挂载插件提供 `loop` Capability，则 `RunTurn` 走外置 Loop（ADR-0003 替换缝）。
- 本票无 tools；工具路径见 ticket 08。system-prompt 注入亦留到后续 Loop 增强票。
- 流式 chunk 用 `EmitTo(req.ID, …)` 归属请求；Host 在 `res` 返回时汇总 `evt`（wire 流式，尚未边收边回调）。
- 方法名：代码用 `llm.complete` + `llm` chunk evt；spec Frame 示例中的 `llm.stream` 以本票为准对齐。
- 无 id 的广播 `evt` 仍被 Host 忽略，留给 Presentation/UI 订阅票。
- Code review 修复：外置 `loop` 替换缝；chunk 方法常量；loop 测试死代码清理。
- CONTEXT.md 尚无「Agent Loop」词条，可由 domain-modeling 后续补。
