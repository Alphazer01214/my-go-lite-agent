# 01 - Manifest autostart + dependsOn �?Discovery 警告

Status: done

**What to build:**

- `plugin.Manifest` 增加 `Autostart bool`、`DependsOn []string`（插件名，`[a-z0-9-]+`，不可自指）�?- 内置插件源侧：agent/session/llm-openai/context-manager �?manifest �?build 脚本输出 `autostart: true`；其�?false�?- Discovery：缺�?README.md �?stderr 警告（不拒载、不�?Errors 硬失败）�?- 单测：manifest 解析/校验；discovery README 警告路径�?
**Done when:** 字段可解析；核插�?dist �?autostart；无 README 仍可 Scan 成功�?