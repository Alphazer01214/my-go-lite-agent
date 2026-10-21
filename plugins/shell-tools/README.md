# shell-tools

`shell-tools` 提供同步进程执行工具 `shell_execute`。

## 注册

Tool `shell_execute` 指向 `shell.execute`，risk 为 `90`，tags 为 `["shell"]`。启动时调用 `tools.sync`，Provider 重启后重新同步。

Provider 接收 `session_id`、`workspace`、`workspace_id`、`call_id` 和 `mode`；执行目录仍以 `workspace` 为锚点。

## 行为

- `command` 和 `args` 直接传给 `exec.Command`，不经过 Shell 解析。
- `cwd` 默认是 Workspace 根目录。相对路径与工作区内的绝对路径都接受；解析后越界返回 `cwd_escape`。
- 工具描述带 `Platform` 和解释器调用示例。`cwd`、`command`、`args` 的 schema 描述同时进入模型上下文。
- 支持环境变量覆盖，不记录环境变量值。
- 默认超时 120 秒，最大 10 分钟。
- 单流输出默认上限为 1 MiB。
- Windows 下非 UTF-8 的控制台输出按当前 ANSI 代码页转换为 UTF-8；其他平台原样返回。
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
