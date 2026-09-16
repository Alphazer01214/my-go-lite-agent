# 自动压缩为默认行为

**Status: accepted**

ADR-0013 与 ADR-0015 两处写「默认 Loop 不做自动压缩」，`phase1-coding-agent/spec.md` 把它列入 Out of Scope，`agent-memory-opt/issues/05` 至今标 `open`——但 agent 插件已实现并接线：`maybeAutoCompact`（plugins/agent/main.go:313）在 `contextProbe`（首个 `context.prepare` 成功后翻转，:938-943）之后，每 Turn 首次 `suggestCompact` 为真时执行 `context.compact` 并把摘要 append 为事实，自第二个 Step 起生效。

**决策：承认现状为正式行为。** supersedes ADR-0013 与 ADR-0015 中「默认 Loop 不做自动压缩」「自动压缩仍 deferred」的表述，并关闭 `agent-memory-opt/issues/05`。

不变的部分（ADR-0013 的核心，继续有效）：压缩仍以日志事实落地——产出是 `context_summary` 事实，derive 只投影 active summary 及其后原文，不删改历史；`context.prepare` 只负责给出 `compactHint`，是否采纳由 Loop 决定。

仍待定（本轮不解决，留 backlog）：

- 阈值沿用 Context Window 的 0.80（context-manager `softBudgetRatio`），未做按模型自适应。
- token 为字符估算而非 provider 精确 usage；ADR-0015 的「provider token 对账」仍是 backlog。
- 每 Turn 最多触发一次（`autoCompacted` 门），未做 Turn 内多轮压缩。

代价：估算偏差可能造成过早压缩（浪费一次 compact 调用）或过晚压缩（撞窗口）。收益：长会话不再依赖用户显式触发，与「拿到就能跑」的定位一致。
