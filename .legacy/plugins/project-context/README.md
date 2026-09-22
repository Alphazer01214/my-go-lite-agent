# project-context

读取 Workspace 里的项目说明文件（`AGENTS.md` / `CLAUDE.md`），作为项目上下文供 Context Manager 组装。

## 提供

- Capability `project-context`
  - `load {workspace}` → `{text, source}`

## 约束

- 单文件上限 32 KiB（`maxBytes`）；按约定顺序取第一个存在者

## Manifest

- 非 autostart；由 Agent Scheme `coding` 拉起
