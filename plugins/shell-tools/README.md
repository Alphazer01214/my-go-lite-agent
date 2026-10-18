# shell-tools

`shell-tools` 提供同步进程执行工具 `shell_execute`。

## 行为

- `command` 和 `args` 直接传给 `exec.Command`，不经过 Shell 解析。
- `cwd` 必须是 Workspace 内的相对目录。
- 支持环境变量覆盖，不记录环境变量值。
- 默认超时 120 秒，最大 10 分钟。
- 单流输出默认上限为 1 MiB。
- 非零退出码是业务失败。

## 取消

Unix 使用进程组，Windows 使用 Job Object。Context 取消或超时会终止整个进程树。

## 配置

`config.json`：

```json
{"allow_shell_interpreters": false}
```

默认拒绝 `cmd`、PowerShell、Bash 等通用解释器。Sandbox 仍会独立检查该高危操作。

完整契约见 [communication.md](communication.md)。
