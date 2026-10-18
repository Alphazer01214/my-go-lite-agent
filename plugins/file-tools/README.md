# file-tools

`file-tools` 提供 Workspace 范围内的文本文件和目录工具。

## 工具

| Tool | Target | 风险 |
|---|---|---|
| `file_read` | `file.read` | low |
| `file_write` | `file.write` | high |
| `file_list` | `file.list` | low |
| `file_stat` | `file.stat` | low |
| `file_search` | `file.search` | low |
| `file_glob` | `file.glob` | low |
| `file_replace` | `file.replace` | high |
| `file_patch` | `file.patch` | high |

## Workspace

Workspace 必须是绝对目录路径。文件参数使用 Workspace 相对路径，统一使用 `/`。Provider 使用 `os.Root` 阻止根外访问。

写入使用同目录临时文件和 rename。`file_patch` 只要有一个编辑失败，就不写回文件。

## 限制

- 读取上限为 16 MiB。
- 写入和编辑上限为 4 MiB。
- `file.replace` 和 `file.patch` 的文件上限为 8 MiB。
- 二进制和非法 UTF-8 文件不能读取或编辑。

完整参数与错误码见 [communication.md](communication.md)。
