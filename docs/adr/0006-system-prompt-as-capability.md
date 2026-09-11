# System Prompt 作独立 Capability

System Prompt 由独立插件提供（Capability `system-prompt`），Agent Loop 在首个 Step 前取用并落入 Session Log 作为模型可见事实。Assembly 可给静态默认值，插件可覆盖。

与 ADR-0003「注入 session / llm / tools / system-prompt 等 Capability」一致：System Prompt 不是 Host 配置字段，也不是旁路通道——它走 Capability 拿取、走 Session Log 落盘、走 derive 进入 Model Context。代价：最小 demo 也需一个 system-prompt 插件（或 Host 内建空实现）。
