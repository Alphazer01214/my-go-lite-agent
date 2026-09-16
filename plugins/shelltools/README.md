# shelltools

跨平台 shell 工具（Tools Plugin，无 PTY）：在 Session Workspace 下执行命令。

## 提供

- Capability `tools`
  - `list` → shell 工具 schema（**非** readOnly，会进入策略裁决）
  - `call` → 在 Workspace 下执行命令

## 约束

- 默认超时 60s（`defaultTimeoutMs`）；单次输出上限 32 KiB（`maxOutput`）
- 危险命令由 sandbox 插件裁决 allow / ask / deny，本插件不内建策略

## Manifest

- 非 autostart；由 Agent Scheme `coding` 拉起
