# 07 — 最小一轮：默认 Loop + fake-llm

**What to build:** 用户提交一条输入后，默认 Agent Loop（在 Host 内）驱动完整聊天轮次：写 Session → 组装 Model Context → fake-llm 流式返回 → assistant 结果落 Session。本票不含工具。

**Blocked by:** 06 — Session 插件与日志不变量

**Status:** ready-for-agent

- [ ] 默认 Loop 使用 Host 内建实现，注入 session/llm（及 loop 所需最小 Capability）
- [ ] 用户输入成为 Session 事实，并出现在下一次 Model Context
- [ ] fake-llm 插件可流式 `evt` chunk，最终 `res` 形成 assistant 消息并追加进日志
- [ ] 一轮结束后 Host 可干净 idle 或退出（按入口约定）
- [ ] 主缝测试：单条用户消息 → 可派生出完整 assistant 回复；全程不依赖真实网络 LLM
