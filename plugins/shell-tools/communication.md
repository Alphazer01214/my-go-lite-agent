# shell-tools 通信契约

本文定义 `shell-tools` 的方法和结果。`shell-tools` 通过 `tools.sync` 注册 `shell_execute`，Target 为自身的 `shell.execute`。

## 0. 注册与 Provider 输入

Tool `shell_execute`：risk `90`，tags `["shell"]`。

`tools.call` 转发的 Provider 输入：

```json
{
  "arguments": {"command": "go", "args": ["test", "./..."]},
  "session_id": "s-1",
  "workspace": "E:/project",
  "workspace_id": "ws-1",
  "call_id": "call-1",
  "mode": "coding"
}
```

Provider 使用 `arguments` 与 `workspace`；其余字段用于审计和后续策略。

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

`command` 必填，是可直接执行的文件名或路径。Shell 内建命令与操作符不会被解释；需要 shell 语义时显式调用解释器，
例如 Windows 上的 `cmd.exe /c ...` 或 `powershell.exe -NoProfile -Command ...`，并受 `allow_shell_interpreters` 控制。

`cwd` 默认 `.`，按 Workspace 根目录解析。相对路径与工作区内的绝对路径都接受；解析后越界返回 `cwd_escape`。
`timeout_ms` 范围是 1 到 600000。`max_output_bytes` 范围是 1 到 4194304。

## 2. 结果

`content`：

```text
exit_code: 0
stdout:
...
stderr:
...
```

Windows 下，若输出字节不是合法 UTF-8，按宿主当前 ANSI 代码页（`GetACP`）解码后再写入 `content`；
`stdout_bytes` / `stderr_bytes` 仍记录原始字节数。其他平台不做转换。

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
