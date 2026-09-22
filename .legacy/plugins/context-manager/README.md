# context-manager

Context Manager：组装 System Prompt，提供 context.prepare / compact / usage / list。

## 提供

- Capability `system-prompt` / `context`
- 命令：`/context-manager usage | list | skills`
- 静态 Prompt Segment：`segments.json`

## 配置

无独立运行 config；skills 目录等由 Workspace 约定提供。

## UI

- `context-manager-status` —— Host 底栏信息区 chip（tokens / context window 占比 / 消息数 / 来源 provider|chars）

## Manifest

- `autostart: true`（核四件之一）
