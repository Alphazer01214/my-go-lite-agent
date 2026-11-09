状态：当前实现

# Shell Tools

Shell Tools 提供 `shell` Capability，暴露同步进程执行工具 `shell_execute`。

## Capability

| 方法 | Tool 名 |
|---|---|
| `shell.info` | 返回全部 Tool 定义 |
| `shell.execute` | `shell_execute` |

Agent 通过 `tool.call` 执行该工具，Tools 根据 `shell.info` 把 `shell_execute` 路由到 `shell.execute`。

## 行为

- `command` 只能是一个可执行文件名或路径，不能包含参数、flag 或 shell 语法；每个参数作为独立的 `args[]` 项传入。
- `command` 与 `args` 直接交给 `exec.Command`，不解析管道、重定向、`&&` 或 shell 内建命令。
- 需要 shell 语法时显式调用解释器，例如 `cmd.exe /c`、`powershell.exe -Command`、`bash -lc`。工具层不拦截解释器，权限裁决留给 sandbox。
- `cwd` 默认 `.`，相对路径以 workspace 为基准；工具层不检查路径越界。
- `env` 覆盖继承的环境变量；不记录变量值。
- `stdin` 可选，原样写入子进程 stdin。
- 默认超时 120 秒，最大 10 分钟；单流输出默认 1 MiB，最大 4 MiB。
- stdout 与 stderr 组成的最终 `content` 超过 1 MiB 时整体截断，并置 `truncated=true`。
- 非零退出码是业务失败；内部超时返回 `is_error=true` 与 `meta.timed_out=true`。
- 取消或超时终止整个进程树：Unix 使用进程组，Windows 使用 Job Object。
- Windows 下非 UTF-8 输出按当前 ANSI 代码页转 UTF-8。

## 调用结果

结果包含 `content`、`is_error`、`truncated` 和 `meta`。

`content` 格式：

```text
exit_code: 0
stdout:
...
stderr:
...
```

`meta` 包含退出码、stdout/stderr 字节数、截断标记、起止时间、耗时、`timed_out`，以及可选的 `stdin_bytes`。

## 配置

无。工具层不做权限或解释器策略；后续由 sandbox 裁决。
