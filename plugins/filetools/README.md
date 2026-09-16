# filetools

工作区文件工具（Tools Plugin）：读 / 写 / 编辑 / 检索，路径一律相对 Session Workspace 解析（ADR-0020）。

## 提供

- Capability `tools`
  - `list` → `read_file`、`write_file`、`edit_file`、`grep`、`glob`
  - `call` `{name, arguments, workspace}` → `{content, additionalContexts}`
- `read_file` 默认只取 500 行（省略 limit 时）——内存优化：工具结果保持小体积

## 行为约定

- `workspace` 由 Agent 在 tools.call payload 注入；插件不猜 cwd
- 越界路径硬拒绝（纵深防御）；真正的产品策略在 sandbox 插件（ADR-0019）

## Manifest

- 非 autostart；由 Agent Scheme `coding` 的 `dependsPlugins` 经 Host `ensurePlugins` 拉起
