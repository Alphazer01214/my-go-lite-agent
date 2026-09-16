# 11 — Presentation Card 形状

**What to build:** 工具/交互插件可发 `type:evt` 的 Presentation Card（从 args/result 纯函数投影）。本票不实现 UI，只定义事件形状并保证回放时卡片可重现。

**Blocked by:** 08 — 工具路径

**Status:** resolved

- [x] Presentation 载荷为结构化 Card（带可辨识类型标签），经 Frame `evt` 发出
- [x] Card 投影不做 I/O、不读时钟/随机；仅依赖 args/result（及可选已持久化 meta）
- [x] 同一 Session 重放时同一工具结果得到相同 Card（主缝可断言）
- [x] 无 Presentation 的 Function 仍可正常工作（可选面）
- [x] UI 消费插件不在本票范围；仅保证事件形状可供后续 UI 插件订阅
- [x] 发 Card 的插件经 `pluginsdk` 的 `Emit` 发送

## Answer

Card 形状：`cap=presentation, method=card` 的 `evt`，payload `{cardType, tool, data}`。Host `collectEvent` 在按 id 过滤前记录广播 Card（`Server.Cards()`）。`echotool` 在成功 `tools.call` 后用 `Emit` 发送 `echo_result` Card——`emitEchoCard` 为纯函数（仅 args+result）。CLI `-cards` 打印本次观察到的 Card。主缝测试：工具路径产出 Card；两次独立 Host 运行同输入 Card 字节级一致；无 Presentation 的 LLM 路径仍 `turn ok` 且无 Card。

## Comments

- UI 消费插件本票不实现；事件形状已可供后续订阅（广播 evt）。
- Card 不含 call id / 时间戳 / 随机数，保证回放可重现。
- 无 Presentation 的 Function（echo、fakellm）不受影响。
- Host 丢弃无 id 的非 presentation evt 的行为未改（留给 UI 订阅票）。
- Code review 修复：纯投影 `echoCard` 与 `EmitCard` 传输拆分；`pluginsdk.Card`/`EmitCard` 暴露作者契约；成功路径只 Emit 一次。
- 「无 Presentation 的 Function」主缝以 fakellm（无 tools）覆盖；带 tools 但不 Emit 的路径结构上允许，未单独立测。
