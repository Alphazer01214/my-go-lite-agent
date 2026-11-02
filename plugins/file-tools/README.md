状态：当前实现

# File Tools

File Tools 提供工作区文件 Tool Provider。

## Capability

File Tools 提供 `file` Capability。

| 方法 | Tool 名 |
|---|---|
| `file.info` | 返回全部 Tool 定义 |
| `file.read` | `file_read` |
| `file.write` | `file_write` |
| `file.glob` | `file_glob` |
| `file.grep` | `file_grep` |
| `file.edit` | `file_edit` |

Agent 通常通过 `tool.call` 执行这些工具。
Tools 根据 `file.info` 把工具名路由到对应 `file` 方法。

## 工作区边界

路径必须位于调用方传入的 workspace 内。
Provider 不接受越界的绝对路径或 `..` 路径。

## 调用结果

结果包含 `content`、`is_error`、`truncated` 和 `meta`。
Agent 将结果写入 `role=tool` 的 Session message Fact。
