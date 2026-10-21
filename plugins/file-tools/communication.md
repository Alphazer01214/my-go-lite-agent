# file-tools 通信契约

本文定义 `file-tools` 的方法、参数和结果。`file-tools` 通过 `tools.sync` 注册八个 Tool，Target 都指向自身的 `file` Capability。

## 0. 注册与 Provider 输入

Tool 风险：

| Tool | Risk | Tags |
|---|---:|---|
| `file_read` | 10 | fs, read |
| `file_list` | 10 | fs, read |
| `file_stat` | 10 | fs, read |
| `file_search` | 10 | fs, read |
| `file_glob` | 10 | fs, read |
| `file_write` | 80 | fs, write |
| `file_replace` | 80 | fs, write |
| `file_patch` | 80 | fs, write |

`tools.call` 转发的 Provider 输入：

```json
{
  "arguments": {"path": "README.md"},
  "session_id": "s-1",
  "workspace": "E:/project",
  "workspace_id": "ws-1",
  "call_id": "call-1",
  "mode": "coding"
}
```

Provider 只解释 `arguments` 与 `workspace`；其余字段用于审计和后续策略。

## 1. 通用边界

Workspace 必须是绝对目录。文件路径必须是 Workspace 相对路径，不能包含盘符、UNC 前缀、NUL 或 `..`。

所有成功结果使用 Tool Result 包络。参数和路径错误使用协议错误，业务失败使用 `is_error: true`。

## 2. file.read

```json
{
  "path": "README.md",
  "start_line": 1,
  "end_line": 80,
  "max_bytes": 65536
}
```

`path` 必填。行号从 1 开始，`end_line` 包含。`meta` 包含 `path`、`size`、`line_start`、`line_end`、`line_count`、`encoding` 和可选 `sha256`。

## 3. file.write

```json
{
  "path": "src/main.go",
  "content": "package main\n",
  "create_dirs": true,
  "overwrite": true,
  "expected_sha256": "64-hex"
}
```

`create_dirs` 默认 `true`，会创建缺失的父目录。显式传入 `false` 且父目录缺失时返回 `path_not_found`，
消息包含父目录路径与 `create_dirs=true` 提示。
`overwrite=false` 且文件存在时返回 `file_exists`。摘要不匹配时返回 `file_conflict`。

## 4. file.list

```json
{
  "path": ".",
  "recursive": true,
  "max_depth": 4,
  "max_entries": 200,
  "include_hidden": true
}
```

不跟随符号链接目录。结果按路径排序。

## 5. file.stat

```json
{"path": "README.md", "hash": true}
```

`meta` 包含 `type`、`size`、`mode`、`mtime`、`is_symlink`、可选 `link_target` 和 `sha256`。

## 6. file.search

```json
{
  "pattern": "tool",
  "path": ".",
  "glob": "**/*.go",
  "regex": false,
  "case_sensitive": true,
  "max_matches": 200,
  "max_files": 2000,
  "max_file_bytes": 1048576,
  "context_lines": 0
}
```

`content` 使用 `path:line:column: text` 格式。二进制、超大文件和符号链接目录会被跳过。

## 7. file.glob

```json
{"pattern": "src/**/*.go", "max_results": 200}
```

`*` 和 `?` 不跨目录，`**` 匹配零层或多层目录。

## 8. file.replace

```json
{
  "path": "README.md",
  "old_text": "old",
  "new_text": "new",
  "replace_all": false,
  "expected_sha256": "64-hex"
}
```

零次匹配返回 `edit_not_found`。多次匹配且 `replace_all=false` 返回 `edit_ambiguous`。

## 9. file.patch

```json
{
  "path": "README.md",
  "edits": [
    {"old_text": "a", "new_text": "b"},
    {"old_text": "c", "new_text": "d", "replace_all": true}
  ],
  "expected_sha256": "64-hex"
}
```

编辑按数组顺序应用。整次修改原子成功或原子失败。

## 10. 错误码

| 错误码 | 场景 |
|---|---|
| `workspace_required` | 缺少 Workspace |
| `workspace_invalid` | Workspace 不是绝对目录 |
| `path_invalid` | 路径格式不合法 |
| `path_escape` | 路径或链接越界 |
| `path_not_found` | 路径不存在 |
| `path_is_directory` | 需要文件但得到目录 |
| `path_not_directory` | 需要目录但得到文件 |
| `file_exists` | 不允许覆盖 |
| `file_too_large` | 文件超过限制 |
| `file_binary` | 不是 UTF-8 文本 |
| `file_conflict` | 摘要不匹配 |
| `line_range_invalid` | 行范围不合法 |
| `edit_not_found` | 文本未找到 |
| `edit_ambiguous` | 文本匹配多次 |
| `file_io` | 其他文件系统错误 |
