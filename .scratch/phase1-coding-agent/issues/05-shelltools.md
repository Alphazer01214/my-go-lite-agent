# 05 — shelltools：三平台 shell 工具

**What to build:** 新建 `plugins/shelltools`：

- provides `tools`；工具名建议 `shell`（或 `run_command`，全仓统一即可）。
- schema：`command`、可选 `timeoutMs`、可选 `cwd`（相对 Workspace，禁止逃逸）。
- 默认解释器：
  - Windows：`powershell.exe -NoProfile -Command`
  - macOS/Linux：`/bin/sh -c`
- 配置（config + 项目 `.liteagent` 覆盖）：`shell`（cmd/pwsh/bash/sh 路径或名）。
- 超时杀进程；stdout/stderr 合并或分字段；结果截断上限（对齐 filetools 量级）。
- 路径参数 slash 入、插件内 `filepath` 归一；`workspace` 来自 call payload。
- 无 PTY、无交互、无流式逐步输出。
- schema **不**标 readOnly；危险性由 policy 管。

**Blocked by:** 01（workspace）、02（多 tools）

**Status:** ready-for-agent

- [x] 三平台默认解释器
- [x] 配置覆盖
- [x] timeout/截断/cwd 约束
- [x] 主缝：当前 OS 跑 `echo`/等价；Windows 与 Unix 各测（build tag 或矩阵）

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- Q4/Q15；路径逃逸交 policy，本插件不私自实现完整 ACL。
