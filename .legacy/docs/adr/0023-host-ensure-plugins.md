# Host 横切面 ensurePlugins

**Status: accepted**

新增 Host 面 `ensurePlugins`（与 `agent.request` / `agent.inject` 同级）：入参插件名列表，对已 Discovery 的名字幂等挂载（含其 Manifest `dependsOn` 闭包），返回未发现/失败名单。Agent 在选定 Scheme 时调用，以落实 `dependsPlugins`；Host **不**解析任何插件业务 config。

Agent Scheme 真源在 agent 插件 config：`dependsPlugins` + `allowedTools`（缺省不按名单过滤，空数组 = 无外部 tool）；无 readOnly 门禁，工具面完全由挂载与名单决定。可热切换；当前 scheme 名记入 Session Log（不进 Model Context）。Web Settings 与 `config.set` 注册 scheme 选项。
