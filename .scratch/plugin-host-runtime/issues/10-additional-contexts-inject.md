# 10 — Additional Contexts 与 inject

**What to build:** Function 可在返回结果时附带 Additional Contexts，由 Loop 在工具结果之后写入 Session 并进入后续 Model Context；插件可经 Host 调用 `agent.inject` 注入模型可见消息且不唤醒空闲 agent。

**Blocked by:** 07 — 最小一轮：默认 Loop + fake-llm

**Status:** ready-for-agent

- [ ] Function 返回的 additionalContexts 出现在对应结果之后的 Session 事实中
- [ ] 下一次模型请求的 Model Context 能从日志重建出这些附加消息
- [ ] `agent.inject` 追加持久可见内容，但不把 idle agent 变成 running（对齐 spec）
- [ ] 插件不得直接改写历史数组；主缝测试断言仅通过 append 路径生效
- [ ] 与 08 工具路径兼容：工具结果 + additionalContexts 顺序正确
