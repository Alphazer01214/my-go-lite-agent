# shell-tools 通信契约

## 1. shell.execute

```json
{
  "command": "go",
  "args": ["test", "./..."],
  "cwd": ".",
  "env": {"CGO_ENABLED": "0"},
  "timeout_ms": 120000,
  "max_output_bytes": 1048576
}
```

`command` 必填。`cwd` 默认 `.`。`timeout_ms` 范围是 1 到 600000。`max_output_bytes` 范围是 1 到 4194304。

## 2. 结果

`content`：

```text
exit_code: 0
stdout:
...
stderr:
...
```

`meta`：

```json
{
  "exit_code": 0,
  "stdout_bytes": 123,
  "stderr_bytes": 0,
  "stdout_truncated": false,
  "stderr_truncated": false,
  "started_at": "RFC3339",
  "finished_at": "RFC3339",
  "duration_ms": 123,
  "timed_out": false
}
```

## 3. 取消

父 Context 取消时返回 `call_cancelled`。内部超时返回业务失败，`meta.timed_out=true`。两者都会终止子进程组或 Job。

## 4. 错误码

| 错误码 | 场景 |
|---|---|
| `workspace_required` | 缺少 Workspace |
| `workspace_invalid` | Workspace 不合法 |
| `cwd_invalid` | `cwd` 不合法 |
| `cwd_escape` | `cwd` 越界 |
| `command_not_found` | 可执行文件不存在 |
| `command_blocked` | 解释器被配置禁用 |
| `env_invalid` | 环境变量不合法 |
| `process_start_failed` | 进程启动失败 |
| `process_timeout` | 内部超时 |
