状态：当前实现

# Git

Git 提供当前工作区的本地状态、分支管理和文本 diff 查看。
插件只执行本地 Git 命令，不做 fetch、pull、push、merge 或 rebase。

## 方法

| 方法 | 说明 |
|---|---|
| `git.status` | 当前分支、上下游、ahead/behind、工作区和暂存区文件 |
| `git.branches` | 本地分支列表和当前分支 |
| `git.diff` | 单个文件的工作区或暂存区 diff |
| `git.checkout` | 切换本地分支 |
| `git.branch.create` | 从当前 HEAD 创建并切换到新分支 |
| `git.branch.delete` | 删除非当前本地分支 |

`workspace` 可省略。
省略时插件调用 `workspace.current`，使用 Session 当前选中的工作区。

## 状态

`git.status` 返回：

```json
{
  "workspace": "E:/project",
  "repo": true,
  "root": "E:/project",
  "branch": "main",
  "upstream": "origin/main",
  "ahead": 1,
  "behind": 0,
  "staged": [
    {"path": "src/a.ts", "status": "M", "insertions": 3, "deletions": 1}
  ],
  "unstaged": [
    {"path": "src/b.ts", "status": "M", "insertions": 2, "deletions": 0},
    {"path": "notes.md", "status": "?"}
  ]
}
```

- `status` 取 `M`、`A`、`D`、`R`、`C`、`T`、`?` 或 `U`。
- 重命名和复制带 `orig_path`。
- 二进制文件带 `binary=true`，没有行数。
- 冲突文件同时出现在 `staged` 和 `unstaged`，`status=U`，并计入 `conflicted`。
- 工作区是仓库子目录时，只返回该子目录下的文件，路径相对工作区。
- 工作区不是仓库时返回 `repo=false`，不返回错误。

## 分支

`git.branches` 返回本地分支，当前分支排在最前。

```json
{
  "workspace": "E:/project",
  "current": "main",
  "detached": false,
  "branches": [
    {"name": "main", "current": true, "upstream": "origin/main", "subject": "init", "committed_at": 1793682665}
  ]
}
```

`git.checkout` 请求：

```json
{"branch": "feature/x"}
```

`git.branch.create` 请求：

```json
{"name": "feature/x"}
```

创建等价于 `git checkout -b`，成功后当前分支即新分支。

`git.branch.delete` 请求：

```json
{"name": "feature/x"}
```

只执行 `git branch -d`。
当前分支、未合并分支和非法分支名都会返回错误，不使用强制删除。

## Diff

`git.diff` 请求：

```json
{
  "path": "src/a.ts",
  "orig_path": "src/old.ts",
  "staged": false,
  "untracked": false
}
```

- `staged=false` 显示工作区相对暂存区的 diff。
- `staged=true` 显示暂存区相对 HEAD 的 diff。
- 未跟踪文件没有可比较的 diff，`untracked=true` 时直接返回空 diff。
- 返回 `truncated=true` 表示 diff 超过 512 KiB。
- 路径必须是工作区内的相对路径，`..` 和绝对路径会被拒绝。

## UI

`git-board` 挂在右侧栏，与 Facts 和 Context 组成 tab。

- 顶部是当前分支卡片，点击展开本地分支列表。
- 分支可切换；非当前分支可删除，删除前需要二次确认。
- `+` 打开新分支输入框，回车或“创建”后创建并切换。
- 工作区和暂存区分成两个 tab，显示文件状态和行数。
- 点击文件展开 diff，新增/删除行和 hunk 有配色。
- 切换会话时跟随 `liteagent:session-changed` 的工作区。
- 回合结束、tab 重新可见或点击刷新时重新读取状态。

## 错误

| 码 | 含义 |
|---|---|
| `git_workspace_missing` | 没有可用工作区 |
| `git_not_repo` | 工作区不是 Git 仓库 |
| `git_unavailable` | 找不到 git 命令 |
| `git_bad_path` | diff 路径越出工作区 |
| `git_bad_branch` | 分支名非法或不能删除当前分支 |
| `git_command_failed` | Git 命令失败，返回 stderr |
| `call_deadline_exceeded` / `call_cancelled` | 调用超时或取消 |

单条 Git 命令超时 15 秒，diff 超时 20 秒。
状态和分支输出上限 4 MiB，diff 输出上限 512 KiB。
插件设置 `GIT_TERMINAL_PROMPT=0`，不会等待用户输入。

## 非职责

- 不执行网络操作：fetch、pull、push。
- 不执行合并、变基、cherry-pick。
- 不暂存、提交或修改文件内容。
- 不支持 bare 仓库和非工作树目录。
