# file-tools

`file-tools` 提供 Workspace 范围内的文本文件和目录工具。

## 工具

| Tool | Target | Risk | Tags |
|---|---|---:|---|
| `file_read` | `file.read` | 10 | fs, read |
| `file_list` | `file.list` | 10 | fs, read |
| `file_stat` | `file.stat` | 10 | fs, read |
| `file_search` | `file.search` | 10 | fs, read |
| `file_glob` | `file.glob` | 10 | fs, read |
| `file_write` | `file.write` | 80 | fs, write |
| `file_replace` | `file.replace` | 80 | fs, write |
| `file_patch` | `file.patch` | 80 | fs, write |

## 注册

启动时调用 `tools.sync`，一次提交全部八个 Tool。Provider 重启后重新 `sync`；tools 用 Generation 隐藏陈旧定义。

## Workspace

Workspace 必须是绝对目录路径。文件参数使用 Workspace 相对路径，统一使用 `/`。Provider 使用 `os.Root` 阻止根外访问。

写入使用同目录临时文件和 rename。`file_patch` 只要有一个编辑失败，就不写回文件。

Provider 接收 `session_id`、`workspace`、`workspace_id`、`call_id` 和 `mode`；路径安全仍以 `workspace` 为准。

## 限制

- 读取上限为 16 MiB。
- 写入和编辑上限为 4 MiB。
- `file.replace` 和 `file.patch` 的文件上限为 8 MiB。
- 二进制和非法 UTF-8 文件不能读取或编辑。

完整参数与错误码见 [communication.md](communication.md)。
