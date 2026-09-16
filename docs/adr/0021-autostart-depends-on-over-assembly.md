# Autostart + dependsOn 取代日常 Assembly

**Status: accepted**

日常挂载真源改为：Manifest `autostart`（默认 false）作 Host 启动根集，加上 Manifest `dependsOn`（插件名）递归闭包；环用 visited 跳过并警告，未发现名字警告跳过，挂载顺序任意（星型经 Host）。内置核四件 `agent` / `session` / `llm-openai` / `context-manager` 标 autostart=true；其余（含 coding 工具）默认 false，由 Agent Scheme 的 `dependsPlugins` 经 Ensure Mount 拉起。

显式 Assembly 文件与 `-assembly` 产品路径废弃：包与测试保留供学习/调试，入口只 warn deprecated 不再作为挂载白名单。插件作者只需声明与其它插件的关系，不再为「场景装配」操心内核文件。代价：可复现场景从单一 JSON 白名单变为「autostart 根 + Scheme dependsPlugins」组合；多 provider 仍全部挂载，收窄靠 Scheme `allowedTools`。

supersedes 日常用法层面的「Assembly 文件 = 挂载真源」（与 ADR-0001 的 Discovery/Assembly 分工仍兼容：发现 ≠ 挂载，只是挂载输入不再来自用户装配清单）。
