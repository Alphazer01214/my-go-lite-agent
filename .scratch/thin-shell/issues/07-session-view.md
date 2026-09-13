# 07 — 主会话视图插件化（渲染）

**What to build:** session 插件第三挂载 session-view 组件（主内容槽位）：重放历史事实（capability 调用）+ 实时消费 presentation/stream——markdown_text / message_text / summary_text / stream 的渲染语义与 CLI Medium 一致、不变。Shell 的内建聊天渲染**同票移除**；输入框本期暂留 Shell（08 迁移）。

**Blocked by:** 05

**Status:** ready-for-agent

- [ ] 对话显示完全由 session-view 渲染，渲染语义与 CLI Medium 一致
- [ ] 流式输出、Presentation Card、settle 后全量渲染不回归
- [ ] Shell 主内容区不再有内建聊天渲染
- [ ] 历史重放走 capability 调用（/api/history 的删除留给 09）
