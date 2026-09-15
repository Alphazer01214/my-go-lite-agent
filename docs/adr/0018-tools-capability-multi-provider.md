# Tools Capability 允许多提供方

`tools` 从「唯一属主」改为 Host 可汇聚的多提供方 Capability：`tools.list` 扇出合并 schema（按工具名去重），`tools.call` 按工具名路由到注册该名的插件。`session` / `llm` / `loop` 等仍保持单一属主。

Phase 1 需要 filetools、shelltools、skill-manager、todo 等同时贡献工具；原先「多工具装在同一 Tools 插件内」会逼出巨型二进制，违背进程外插件的独立分发。代价是 Host 路由表多一层 tool-name → plugin 映射，Assembly 启动时若两插件注册同名工具则 fail-loud。
