# 11 — Presentation Card 形状

**What to build:** 工具/交互插件可发 `type:evt` 的 Presentation Card（从 args/result 纯函数投影）。本票不实现 UI，只定义事件形状并保证回放时卡片可重现。

**Blocked by:** 08 — 工具路径

**Status:** ready-for-agent

- [ ] Presentation 载荷为结构化 Card（带可辨识类型标签），经 Frame `evt` 发出
- [ ] Card 投影不做 I/O、不读时钟/随机；仅依赖 args/result（及可选已持久化 meta）
- [ ] 同一 Session 重放时同一工具结果得到相同 Card（主缝可断言）
- [ ] 无 Presentation 的 Function 仍可正常工作（可选面）
- [ ] UI 消费插件不在本票范围；仅保证事件形状可供后续 UI 插件订阅
- [ ] 发 Card 的插件经 `pluginsdk` 的 `Emit` 发送
