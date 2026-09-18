# 02 — Session Permission Mode

Status: resolved

**What to build:** ADR-0033：session 元数据 `permissionMode` + create/setPermissionMode；agent 传参；sandbox 模式档；Web New-session/会话头 UI。

**Acceptance:**
- [x] 三档裁决表（规则优先可放宽）
- [x] 默认 workspace_write；会话中可改
- [x] Subagent 继承父模式
- [x] `session_meta` 记录变更

**Answer:** `plugins/session` Meta + `setPermissionMode`；`plugins/agent` sessionPermissionMode → policy.decide；`plugins/sandbox` modeProfile；Session View welcome chips + 会话头 select。测试 `plugins/session/mode_test.go`、`plugins/sandbox/main_test.go`。
