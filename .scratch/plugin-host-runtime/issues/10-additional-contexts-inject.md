# 10 — Additional Contexts 与 inject

**What to build:** Function 可在返回结果时附带 Additional Contexts，由 Loop 在工具结果之后写入 Session 并进入后续 Model Context；插件可经 Host 调用 `agent.inject` 注入模型可见消息且不唤醒空闲 agent。

**Blocked by:** 07 — 最小一轮：默认 Loop + fake-llm

**Status:** resolved

- [x] Function 返回的 additionalContexts 出现在对应结果之后的 Session 事实中
- [x] 下一次模型请求的 Model Context 能从日志重建出这些附加消息
- [x] `agent.inject` 追加持久可见内容，但不把 idle agent 变成 running（对齐 spec）
- [x] 插件不得直接改写历史数组；主缝测试断言仅通过 append 路径生效
- [x] 与 08 工具路径兼容：工具结果 + additionalContexts 顺序正确

## Answer

`tools.call` 结果支持 `additionalContexts: [{role,content}]`；Loop 在 `tool_result` 落盘后立即 `session.append` 这些 message 事实（US16）。Host 新增 `agent.inject`（`serve.AgentInject`）：接受单条 `{role,content}` 或 `{messages:[]}`，仅经 Session append 写入，不触发默认 Loop（US17）。插件可经星型 `cap=agent, method=inject` 调用；CLI：`-agent-inject`。Fixture：`echotool` 在 text=ctx 时返回附加上下文；`agentprobe` 支持 `cap=inject`。主缝测试覆盖：tool 结果后附加上下文顺序、CLI inject 不唤醒 turn、插件星型 inject。

## Comments

- inject 只追加；不改写历史，也不启动 Loop。空闲 agent 保持空闲。
- additionalContexts 默认 role=system；缺省时 Loop 补 system。
- 仅通过 `session.append` 生效——Session 仍是唯一真源（ADR-0002）。
- CLI 操作顺序：append → inject → turn → invoke → derive → query → request（保证 derive 看到注入内容）。
