# Assistant 结果仅在 LLM 成功后落入 Session Log

System Prompt 与用户消息在调用 LLM 前落入 Session Log（否则 Model Context 无法从日志重建，违背 ADR-0002）。**Assistant 结果仅在 `llm.complete` 成功返回后才 append**；失败或取消不得写入半截 assistant 事实。

对齐 deepseek-harness 的精神：模型「说出了什么」必须以成功 settle 为准。v1 仍用单方法 `llm.complete`；显式两阶段 `prepareCall` 等真实 provider adapter 开票时再定。
