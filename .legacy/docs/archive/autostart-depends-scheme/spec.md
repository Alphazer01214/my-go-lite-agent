# Autostart + dependsOn + Agent Scheme

## Intent

插件作者只声明与其它插件的关系；日常挂载不再依赖 Assembly 白名单。Host 启动 = Autostart 根 + dependsOn 闭包；Agent Scheme 经 ensurePlugins 拉起场景工具并用 allowedTools 决定模型可见 tool 面。

## Decisions (grilling 定案)

见 ADR-0021 / 0022 / 0023 与 CONTEXT.md：Plugin Readme、Autostart、Plugin Dependency、Agent Scheme、Ensure Mount。

### 关键规则摘要

| 项 | 规则 |
|----|------|
| Plugin Readme | 作者自撰、结构自由；缺失 Discovery 警告，不拒载 |
| Autostart | Manifest bool，缺省 false；核四件 true |
| dependsOn | Manifest 插件名数组；递归、防环、未发现警告跳过、任意挂载序 |
| consumes | 弱校验；缺 provider → 警告 + degraded（不进 registry） |
| Assembly | 产品路径停用；包/测试保留；`-assembly` warn deprecated |
| Ensure Mount | Host `ensurePlugins`；Agent 选 Scheme 时调用 |
| Scheme | agent config：`defaultScheme` + `schemes.{name}.{dependsPlugins,allowedTools,…}` |
| allowedTools | 缺省不过滤；`[]` = 无外部 tool；无 readOnly 门禁 |
| 热切换 | config.set / Web Settings；下一 Turn 生效；scheme 名入 Session Log |
| 卸载 | v1 不卸载进程；unload 另票 |
| 多 provider | 全部挂载；收窄靠 allowedTools / dependsPlugins |
| 入口 | `-plugins` 必填；新增 `-scheme` 覆盖 defaultScheme |

## Out of scope (v1)

- 引用计数卸载 / `/lp unload`
- config 覆盖 Manifest dependsOn
- 读工具自动推导 chat 名单（必须显式 allowedTools）
- 懒 spawn（策略乙仅作后续增强）
