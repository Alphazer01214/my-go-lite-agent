状态：当前实现

# Python

Python 提供 `python` Capability，暴露 `python_execute`。工具通过 `shell` Capability 执行，不自己启动进程。

## Capability

| 方法 | Tool 名 |
|---|---|
| `python.info` | 返回全部 Tool 定义 |
| `python.execute` | `python_execute` |

Agent 通过 `tool.call` 执行该工具，Tools 根据 `python.info` 把 `python_execute` 路由到 `python.execute`。

## 输入

`code` 与 `path` 二选一。

- `code`：通过 `python -` 从 stdin 执行；`args` 成为 `sys.argv[1:]`。code 模式不能读取 stdin。
- `path`：运行指定文件；相对路径以 workspace 为基准，绝对路径直接使用；`args` 传给脚本。
- `cwd` 默认 `.`；`env` 覆盖环境变量；`timeout_ms` 与 `max_output_bytes` 传给 shell-tools。
- 工具层不检查路径越界；权限裁决留给 sandbox。

## 解释器

`config.json` 的 `interpreter` 优先；为空时按平台探测：

| 平台 | 顺序 |
|---|---|
| Windows | `python` → `python3` |
| 其他 | `python3` → `python` |

启动时探测一次。找不到解释器仍会提供工具，调用时返回 `interpreter_not_found`。

## 调用结果

原样返回 shell-tools 的 `content`、`is_error`、`truncated` 和 `meta`。
`meta` 增加 `mode` 与 `interpreter`，path 模式再增加 `path`。

## 错误

| 错误码 | 场景 |
|---|---|
| `bad_arguments` | code/path 同时缺失或同时出现、code 含 NUL 或超限、args 含 NUL |
| `path_invalid` | 脚本路径不合法 |
| `path_not_found` | 脚本不存在 |
| `path_is_directory` | 脚本路径是目录 |
| `interpreter_not_found` | shell-tools 找不到解释器 |
| `shell_unavailable` | shell-tools 未安装或未启动 |

shell-tools 的 `cwd_invalid`、`env_invalid`、`process_start_failed` 等错误原样透传。

## 配置

`config.json`：

```json
{"interpreter": ""}
```

`interpreter` 可以是 PATH 中的名字，也可以是绝对路径。
