# Core Plugin Contracts

`core-plugin-contracts.schema.json` 是 `agent`、`session` 和 `llm-openai` 跨插件 JSON 的规范来源。

Go 类型由 `types.go` 提供。插件内部保留原有类型名作为兼容入口，再映射到本包类型。兼容入口不得发展出独立字段语义。

当前覆盖：

- `loop.turn` 与 `loop.cancel`；
- `llm.complete`、`llm.cancel` 与 `llm.chunk`；
- `session.stream.update`；
- Turn、LLM 和 Stream 共用的消息、工具调用及 usage。

修改契约时先改 Schema，再同步 Go 类型、插件适配层和通信文档。`schema_test.go` 检查 Schema 与 Go 字段保持一致。
