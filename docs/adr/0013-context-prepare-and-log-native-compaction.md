# Context Prepare 收口模型上下文；压缩以日志事实落地

模型可见内容（System Prompt、tools schema、选窗后的历史）统一由 Context Manager 的 `context.prepare` 产出，Loop 只编排调用。压缩不改写 Session Log：compact 产出 Context Summary 新事实 append 入日志，`session.derive` 投影 active summary 及其后原文；System Prompt 在 log 中可有多条审计痕迹，但 Model Context 只保留最后一条（内容未变则不重复 append）。Host 暂仍内嵌默认 Loop，后续再按 Agent「模式」解耦。

与 ADR-0002 同构：选择与压缩必须可从日志重建。拒绝在 llm 调用前旁路改写 messages（那会削弱不变量）。代价：session.derive 规则变复杂，且 compact 触发依赖 Loop 预算判断。
