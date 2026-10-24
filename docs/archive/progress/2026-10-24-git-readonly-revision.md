# git 只读方案修订与 plan 状态（2026-10-24 二次讨论）

> 状态：待实现（git 已收窄为只读方案，代码尚未落地；plan 待按新约束重议）
> 取代：[2026-10-24-git-sidebar-plan-handoff.md](2026-10-24-git-sidebar-plan-handoff.md) 的 **git 全部内容**（原文第 1、2.2、4 节、第 6 节表中 git 行、第 7 节阶段 2/3、第 9.1 节）。原文 plan 部分（第 5 节）仍是唯一设计记录，但重议前先读本文第 5 节的约束冲突，不要照做。
> 关联：[15-git-tools.md](../../design/plugins/15-git-tools.md)、[webui.md](../../platform/webui.md)

## 0. 修订原因与硬约束

本次讨论重新定位了 git 的用途：

1. **git 收窄为只读**。核心用途是查差分、查 git 仓库信息；必要时以只读工具向 agent 提供结构化 git 信息。
2. **git 写操作不建模进插件**。LLM 自己会写 git 命令，需要 git 操作（提交、切分支、暂存等）时自行调用 shell tool 执行 `git` 命令。
3. **硬约束：git 实现不修改 host 与其他任何插件**（sandbox、session、tools、model-context 等一律不动）。
4. **路径直接使用 session 提供的 workspace**，不做额外路径抽象。

## 1. 目标

1. WebUI 右侧 Git tab：只读展示分支/状态/分组文件/最近提交/本地分支，点击文件看 diff。
2. 两个只读工具 `git_status`、`git_diff`：agent 必要时查询结构化 git 信息。
3. 把握时机自动读取：进入工作区、挂载、重连、切 tab、聚焦时自动刷新，不需要用户手动。

## 2. 代码事实（本次讨论逐条核实）

| 事实 | 出处 | 对设计的影响 |
|---|---|---|
| seam 是消费方声明制：消费方在自己 Manifest 的 `consumes` 声明并自己注册 seam handler | `internal/host/pipeline.go:50`、`plugins/sandbox/main.go:81` | git 方法进不了 sandbox seam，除非改 sandbox ⇒ 与硬约束冲突 ⇒ 写操作不建模 |
| sandbox 已消费 `shell.execute` | `plugins/sandbox/plugin.json` consumes | LLM 经 shell 跑 git 写命令时天然有裁决、确认面板、审计 ⇒ 写操作交给 shell 路径是完备的 |
| `tools.call` 对 Provider 返回做 `ParseResult` 校验 | `plugins/tools/tools_methods.go:122` | `git.*` 方法统一返回 `Result{content,is_error,truncated,meta}` |
| `session.workspace.current` 已存在 | `plugins/session/main.go:195` | workspace 来源，UI 直调即可；原文的 `session.current` 新增计划删除 |
| 无跨插件事件广播 | 平台事实 | "进入工作区"的时机靠轮询 + 组件生命周期把握 |
| 本目录只增不改 | 本目录 README | 本文以新记录 + 取代标注生效，不覆写原文 |

## 3. git-tools 插件（只读）

### 3.1 命名与 Manifest

- 目录 `plugins/git-tools/`，插件名 `git-tools`，能力 `git`。
- UI 组件 `git-tools-panel`，Mount ID `panel`，Slot `right`，`static tabTitle = "Git"`，`static tabOrder = 20`。

```json
{
  "name": "git-tools",
  "version": "0.1.0",
  "protocol": 1,
  "entry": "git-tools",
  "autostart": true,
  "provides": ["git"],
  "requires": ["session", "tools"],
  "depends_on": ["tools"],
  "ui": {
    "entry": "ui/main.js",
    "components": [
      {"tag": "git-tools-panel", "mount_id": "panel", "slot": "right"}
    ]
  }
}
```

`session` 是弱依赖且只在 UI 侧：浏览器调 `session.workspace.current` 取工作区；服务端不调 session。缺失/失败时面板显示空状态。

### 3.2 载荷约定

方法接受 ProviderInput 形状（与 file-tools 一致）：

```json
{"workspace": "E:/repo", "workspace_id": "ws-1", "session_id": "s-1", "call_id": "call-1", "arguments": {}}
```

- agent 侧由 `tools.call` 注入外层字段。
- UI 直调只读方法时包同样形状；`session_id` 可省略（仅审计元数据，git 不依赖）。
- 路径参数一律相对 workspace。

### 3.3 只读方法与工具

| 方法 | arguments | 工具 | risk | parallel_safe |
|---|---|---|---|---|
| `git.status` | `{}` | `git_status` | 10 | true |
| `git.diff` | `{path?, staged?}` | `git_diff` | 10 | true |
| `git.log` | `{limit?}` | 不暴露 | — | — |
| `git.branches` | `{}` | 不暴露 | — | — |

工具只保留两个：agent 需要的是"结构化状态"和"结构化差分"；log/branches 用 shell 的 `git log` / `git branch` 兜底即可，UI 直调方法。若后续发现 agent 高频查 log，再补 `git_log` 工具（一次 `tools.sync` 即可）。

`git.status` 的 `meta` 至少包含：

```json
{
  "revision": "<内容摘要>",
  "root": "E:/repo",
  "branch": "main",
  "detached": false,
  "upstream": "origin/main",
  "ahead": 1,
  "behind": 0,
  "staged": [{"path": "src/a.go", "status": "modified"}],
  "unstaged": [],
  "untracked": [],
  "conflicted": [],
  "in_merge": false,
  "in_rebase": false,
  "truncated": false,
  "total": 0
}
```

**revision 修正**（对原文 4.4）：`revision` 不是 commit hash，而是 **status 输出的内容摘要**（如 SHA-256(porcelain 输出)）。commit hash 不随工作区变化，暂存/改文件不会改变它，UI 去重会漏重绘。UI 用 `revision` 判"内容不变不重绘"。

### 3.4 Result 形状

所有方法返回 `Result{content, is_error, truncated, meta}`（`tools.call` 校验要求）：

- `content`：模型友好紧凑文本（如 `main ↑1 · 2 staged, 1 unstaged, 2 untracked`）。
- `meta`：3.3 的结构化对象，UI 直调时读 `meta`。
- `truncated`：输出超限时 true，保留截断前完整行。

### 3.5 Git 执行约束

- 只用 `exec.Command("git", args...)`，不拼 shell；`git -C <workspace> ...` 启动，先 `rev-parse --show-toplevel` 找仓库根。
- status：`git status --porcelain=v2 -z --branch --untracked-files=all`，按 NUL 解析；rename/copy 记录带两个路径，文件名可含换行。
- diff：`git diff --no-color --no-ext-diff --no-textconv --unified=3`，staged 加 `--cached`。
- log：NUL 分隔格式，限制返回条数。
- env：设 `GIT_OPTIONAL_LOCKS=0`、`GIT_TERMINAL_PROMPT=0`、`GIT_PAGER=cat`；过滤 `GIT_DIR`、`GIT_WORK_TREE`、`GIT_INDEX_FILE`。
- `--literal-pathspecs` + `--`；路径拒绝绝对路径、`..`、NUL、控制字符、Windows 保留设备名。
- 超时：读命令 10–15 秒。输出上限：status 约 2 MiB、diff 约 512 KiB、log 约 256 KiB，超限置 `truncated`。
- `in_merge` / `in_rebase` 用 `git rev-parse --git-path MERGE_HEAD / rebase-merge` 探测，比读 status 可靠。
- 路径口径：内部一律 repo-root 相对（git 输出本来如此）；边界换算为 workspace 相对；**落在 workspace 之外的条目直接丢弃**。

### 3.6 错误码（只读子集）

`git_not_found`、`git_not_repository`、`git_workspace_invalid`、`git_path_invalid`、`git_command_failed`、`git_output_too_large`。

原文的 `git_commit_empty`、`git_worktree_dirty`、`git_conflict` 随写操作一并删除。

### 3.7 UI（只读面板）

```text
main ↑1 ↓0                    [刷新]
staged 2 · unstaged 1 · untracked 2
▾ 已暂存
  M src/a.go
▾ 未暂存
  M src/b.go
▾ 未跟踪
  ? notes.md
▾ 最近提交
  abc123 message
▾ 本地分支
  main  develop
```

- 点击文件打开右侧 diff 抽屉（宽约 760px）：hunk、行号；untracked 提示"新文件，暂无 diff"；`Binary files … differ` 显示占位。
- 分支行展开本地分支列表（只读）。
- **没有任何写按钮、没有提交框**。
- 错误在面板内联显示，不弹 toast。
- 空状态三态：无 workspace / 非 Git 仓库 / 找不到 git 命令。

### 3.8 时机（自动读取）

"session 进入某个工作区"没有事件可用（不改 session），用既有时机拼出来：

| 时机 | 动作 |
|---|---|
| 组件 `connectedCallback` / `resync()` | 立即 `session.workspace.current` + `git.status` 探测 |
| 5 秒轮询 | 同节奏轮询 `session.workspace.current` 与 `git.status`；workspace_id 变化 → 清缓存、重新找仓库根、立即刷新 |
| tab 激活 / window focus | 立即刷新 |
| 手动刷新按钮 | 立即刷新 |
| agent 需要时 | `git_status` / `git_diff` 工具主动查询 |

所有渲染以 `revision` 去重。多标签页不需要特殊处理：workspace 选择是 session 注册表的全局状态，各页面轮询到的一致。

### 3.9 测试

- 纯函数单测：porcelain v2 -z 解析（rename 双路径记录、换行文件名、无 HEAD 的初始仓库、`# branch` 头含 ahead/behind）、路径换算与拒绝规则、截断按完整行。
- 集成测（`t.TempDir` 造真实仓库，`exec.LookPath("git")` 缺失 `t.Skip`，不伪造输出）：status/diff/log/branches 正确性、非仓库与无 git 的提示、二进制 diff、大输出截断。
- 验收命令：`go test ./...`、`go vet ./...`、`node --test internal/webui/*.test.mjs`、`bash scripts/build.sh --verify`。

## 4. 右侧自动 tab（平台层，保留原文方案）

原文第 3 节方案不变：Custom Element 静态属性 `tabTitle / tabOrder / overlay`，loader 对 `right` Slot 的非 overlay Mount 自动分组，切换只改 `hidden`，tab 记忆存 `localStorage`，组件保持挂载继续收推送。

一个待确认点：原文要求给 `session-trace` 加静态字段、给 `sandbox-confirm` 加 `overlay = true`，这两处是**其他插件的 UI 代码**，与硬约束存在字面冲突。若要求插件目录也冻结，退化方案（零插件改动）：overlay 名单由 host 侧 loader 配置给出（如 `overlayTags: ["sandbox-confirm"]`），tab 标签缺省用插件名。两者行为等价，落地时二选一并在 `webui.md` 写明。

`git-tools-panel` 自己的静态字段属于 git 插件自身代码，不受约束影响。

## 5. plan：状态与约束冲突

本轮只讨论了 git。**plan 尚未重议**；原文第 5 节仍是唯一设计记录，但其中四点与"不修改其他插件"硬约束冲突，重议前不要照做：

| 原文设计 | 冲突 | 重议方向（未定） |
|---|---|---|
| 新增 `session.current` 方法 | 改 session 插件 | 用 `session.workspace.current` + UI 状态自行解析 |
| session UI 派发 `liteagent:session-changed` 事件 | 改 session 插件 UI | 改为轮询（git 面板同款时机表） |
| session trace 的 `factTypeLabels` 加 `plan_update` | 改 session 插件 UI | Fact `type` 原样透出，plan 自己的悬浮窗渲染 |
| model-context `prepare` 读 `plan.current` 注入 | 改 model-context 插件 | 待议：可能改为 plan 提供工具/上下文的其他注入面 |

不冲突、可保留的部分：`update_plan` 工具、`plan.write` / `plan.current` 方法、Session Fact Log 作为真源（`session.append` 是公共契约，不是改插件）、悬浮窗形态、plan 与 task（13-task.md）的关系标注。

下次讨论 plan 时以本节约束为准。

## 6. 实施顺序

### 阶段 A：右侧自动 tab（平台层）

- [ ] loader 读取 `tabTitle / tabOrder / overlay`（或 overlayTags 兜底，见第 4 节）。
- [ ] right Slot 非 overlay Mount 自动分组；切换只改 hidden；localStorage 记忆；移除后回退。
- [ ] loader 测试与 CSS；`webui.md` Component Interface 更新。

验收：session trace 正常、sandbox 浮层不参与 tab、刷新后 tab 记忆、插件启停后 Mount 对齐不重建存活组件。

### 阶段 B：git 只读后端

- [ ] 插件骨架、Manifest、wire、runner（env 清洗、超时、输出上限）、paths（换算与校验）。
- [ ] porcelain v2 -z 解析；diff/log/branches。
- [ ] `git_status` / `git_diff` 工具注册与 `tools.sync`；revision 内容摘要。
- [ ] 构建清单（`scripts/shipped-plugins.conf` core 增加 `git-tools`）与 README/communication。

验收：临时仓库 status/diff/log/branches 正确；非仓库、无 git 有明确提示；大 diff 与二进制不炸；工具调用返回紧凑 `content` + 结构化 `meta`。

### 阶段 C：Git 面板

- [ ] 只读面板、diff 抽屉、分支列表、空状态三态。
- [ ] 时机表（3.8）：挂载/重连/轮询/focus/tab 激活，revision 去重。
- [ ] 浏览器手工回归：工作区切换自动跟随、diff 抽屉、错误显示。

## 7. 与原文的差异对照

| 项 | 原文（2026-10-24 初版） | 本文 |
|---|---|---|
| git 范围 | 含写操作 | **只读** |
| 写操作归属 | 插件工具 stage/unstage/commit/checkout | LLM 经 shell tool 跑 git 命令 |
| sandbox | consumes 四个写方法 + seam handler | **不涉及** |
| UI | 写按钮、提交框、分支切换 | 纯只读展示 |
| 工具数 | 7 个 | 2 个（`git_status`、`git_diff`）；log/branches 仅方法 |
| UI 写路径 | 直调 git 写方法经 sandbox | 不存在 |
| revision | commit hash | status 内容摘要 |
| `session.current` | 新增 session 方法 | 不加，用 `session.workspace.current` |
| 选择事件 | session UI 派发 CustomEvent | 不加，轮询把握时机 |
| 错误码 | 9 个（含写） | 6 个（只读子集） |

## 8. 风险与开放点

1. **LLM 用 shell 跑 git 写命令的纪律**：这是用户明确接受的分工（sandbox 已消费 `shell.execute`，裁决/确认/审计在那条路上是完备的）。git 插件不兜底写安全。
2. **UI 无写入口**：用户在面板里不能直接提交/暂存，需要时让 agent 代劳。已接受。
3. **revision 摘要成本**：每次轮询对最多 2 MiB 的 status 输出做摘要，5 秒一次可接受；必要时可先比对输出长度再算摘要。
4. **15-git-tools.md 冲突扩大**：设计稿仍是含写操作的全量契约。落地时必须改写为只读方案或标注被本文取代，否则后续会话按旧契约实现。
5. **loader overlay 兜底**：见第 4 节，落地时二选一并写进 `webui.md`。

## 9. 关键文件地图

| 内容 | 位置 |
|---|---|
| 右侧 Slot | `internal/webui/static/index.html` |
| Mount Loader（自动 tab） | `internal/webui/static/loader.mjs` |
| Shell 样式 | `internal/webui/static/shell.css` |
| workspace 来源 | `plugins/session/main.go`（`session.workspace.current`） |
| 工具注册契约 | `plugins/tools/communication.md`、`plugins/file-tools/wire.go`（同构参考） |
| 构建清单 | `scripts/shipped-plugins.conf`、`scripts/build.sh`、`scripts/build.ps1` |
| Git 设计稿（待修正） | `docs/design/plugins/15-git-tools.md` |
| WebUI 平台契约 | `docs/platform/webui.md` |
