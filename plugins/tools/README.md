状态：当前实现

# Tools

Tools 聚合 Tool Provider，并提供统一的工具目录和执行入口。

## 方法

| 方法 | 说明 |
|---|---|
| `tool.list` | 返回当前 Tool 定义和 catalog generation |
| `tool.call` | 按工具名路由到对应 Provider 方法 |

## Tool Provider

Tools 通过各 Capability 的 `info` 方法收集 Tool 定义。
Provider 返回 `name`、`description`、`parameters` 和执行 `method`。

当前 Provider：

| Capability | Provider | Tool |
|---|---|---|
| `file` | `file-tools` | `file_read`、`file_write`、`file_glob`、`file_grep`、`file_edit` |
| `shell` | `shell-tools` | `shell_execute` |
| `python` | `python` | `python_execute` |
| `skill` | `prompter` | `skill_read` |
| `mcp` | `prompter` | `mcp_call` |
| `subagent` | `subagent` | `subagent` |

Tools 依赖 Provider 先启动，避免目录在 Provider 就绪前被固定为空。

## 配置

`config.json` 的 `capabilities` 决定拉取哪些 Provider，当前为 `["file", "shell", "python", "skill", "mcp", "subagent"]`。
构建只在文件缺失时从 `config.example.json` 落种子；已有的 `config.json` 不会被覆盖。

## 调用结果

`tool.call` 返回 `content`、`is_error`、`truncated` 和 `meta`。
Tools 校验结果结构后返回 Agent。

## 非职责

不组装模型请求；不保存 Session Facts；不执行文件或命令本身。
