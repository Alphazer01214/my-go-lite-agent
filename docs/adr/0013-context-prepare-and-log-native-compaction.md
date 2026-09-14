# Context Prepare 收口模型上下文；压缩以日志事实落地

模型可见内容以 `session.derive` 为真源（ADR-0002）。Context Manager 经 `context.prepare` 观测/校验该批 messages，并提供 usage 与 listContext（查看进入模型的内容）。压缩不改写 Session Log：`context.compact` 产出摘要文本；若要进入模型，须由调用方 append 为新的 `context_summary` 事实，derive 投影 active summary 及其后原文。**默认 Loop 不做自动压缩。**

System Prompt 在 log 中可有多条审计痕迹，但 Model Context 只保留最后一条（内容未变则不重复 append）。

代价：长会话需显式触发 compact；自动策略留待后续票。

