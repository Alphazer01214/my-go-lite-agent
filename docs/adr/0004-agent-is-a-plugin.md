# Agent 作插件，默认实现在 Host

Agent 是一个 Plugin：消费 llm、session、tool 等 Capability 并执行 Agent Loop。Host 内建默认 Agent（即 ADR-0003 的默认 Loop），外部 Agent 插件经同一 Capability 名可替换。

与「一切皆插件」同构：Agent 不是 Host 特权对象，而是可发现、可组装、可替换的插件。Host 仍强制 Session Log 不变量（ADR-0002）与 agent/request、agent.inject 这两条横切面——它们属于 Host，不属于某个 Agent 实现。代价：默认路径上 Agent 逻辑在 Host 进程内，与外置 Agent 插件共享契约但不共享进程。
