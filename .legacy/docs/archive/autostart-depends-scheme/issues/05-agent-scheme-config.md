# 05 - Agent Scheme config �?Loop 工具�?
Status: done

**What to build:**

- agent 插件实现 Config Capability：`config.schema` / `get` / `set` / `reload`�?- 配置形状：`defaultScheme`；`schemes.<name>.{dependsPlugins[], allowedTools[], maxSteps?, runSubagent?, todo?}`�?- 内置默认：`chat`（allowedTools 显式只读名单，dependsPlugins 可空�?filetools）、`tool_calling`（不过滤 / �?dependsPlugins 工具全家）、`coding`（coding 工具 dependsPlugins + allowedTools）�?- Turn 前：ensurePlugins(dependsPlugins)；收�?tools.list 后过滤：allowedTools 省略=不过滤，[]=无外�?tool；无 readOnly 门禁�?- 热切换：set defaultScheme 后下一 Turn 生效；append scheme �?Session Log 事实（不�?derive �?model-visible，除非已�?meta 约定）�?- `/agent config …` �?Web Settings 表单�?
**Done when:** chat �?tool_calling 行为可测分离；改 scheme 无需重启 Host�?