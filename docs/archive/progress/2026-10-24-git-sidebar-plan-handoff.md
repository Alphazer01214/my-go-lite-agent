# git 侧边栏与 plan 悬浮窗交接

> 状态：待实现（2026-10-24 设计已确认，代码尚未落地）
> 说明：本文是一次设计讨论的共识记录。实现时以本文为准；与现有设计稿的差异见第 9 节，落地时必须同步修正那些文档来源。
> 关联：[15-git-tools.md](../../design/plugins/15-git-tools.md)、[13-task.md](../../design/plugins/13-task.md)、[webui.md](../../platform/webui.md)

## 1. 目标与已确认决策

本轮做两件事：

1. 在 WebUI 右侧增加 Git 侧边栏，与现有 Session trace 切换。
2. 增加 `plan` 插件，作为长任务的进度工具，并提供可拖拽悬浮窗。

用户已拍板的五个决策：

| 决策 | 结论 |
|---|---|
| Git 范围 | 包含写操作 |
| Git 写操作入口 | UI 与 agent 都可发起 |
| 右侧切换 | 右侧非浮层 Mount 自动成 tab，不改 Manifest |
| plan 存储 | 写入 Session Fact Log，并注入模型 runtime context |
| plan 窗口 | 可拖拽悬浮窗，默认右下，可折叠 |

## 2. 当前代码事实

### 2.1 右侧 Slot

- 右侧当前直接挂载 `session-trace`，见 [index.html:33](../../../internal/webui/static/index.html#L33)。
- `sandbox-confirm` 也声明在 `right`，但组件内部 `position: fixed`，不占布局。
- Loader 目前只做 `slot.append(element)`，没有 tab 概念，见 [loader.mjs:82](../../../internal/webui/static/loader.mjs#L82)。
- Bootstrap 的 Mount 只有 `tag / slot / entry`，见 [bootstrap.go:27](../../../internal/webui/bootstrap.go#L27)。

### 2.2 工具与 workspace

- `tools.call` 会把 `session_id / workspace / workspace_id / call_id / mode` 传给 Provider，见 [turn_tools.go:123](../../../plugins/agent/turn_tools.go#L123)。
- `session.workspace.current` 已存在，见 [main.go:195](../../../plugins/session/main.go#L195)。
- 目前没有跨插件事件广播；Host 只转发带 id 的 Event，UI 之间只能靠浏览器侧 CustomEvent。

### 2.3 Session Fact Log

- Session 已是 append-only 真源，`session.append` 接受任意非空 `type`，见 [session_ops.go:81](../../../plugins/session/session_ops.go#L81)。
- 非 message Fact 会进入 trace，不会进入 `session.derive` 的模型消息序列。
- `session.append` 对非 message Fact 也会推进 Chat/Trace 的 `through_seq`，不会破坏后续增量，见 [ui_publish.go:22](../../../plugins/session/ui_publish.go#L22)。

### 2.4 工作区状态

- 分支：`plugins`。
- HEAD：`6731a6f`。
- 工作区已有与本设计无关的未提交修改：`README.md`、`README.zh-CN.md`、`docs/platform/internal_webui.md`、`docs/platform/webui.md`、`internal/webui/plugins.test.mjs`、`internal/webui/server_test.go`、`internal/webui/settings.test.mjs`、`internal/webui/static/app.js`、`internal/webui/static/index.html`、`internal/webui/static/plugins.mjs`、`internal/webui/static/settings.mjs`、`internal/webui/static/shell.css`、`site/data.js`。
- 这些修改不要回退、不要覆盖。本文是新增文件，未提交。

## 3. 右侧自动 tab

### 3.1 组件接口

不改 Manifest、Bootstrap、Registry。给 Custom Element 增加三个可选静态属性：

```js
class SessionTrace extends HTMLElement {
  static tabTitle = "会话";
  static tabOrder = 10;
}

class GitPanel extends HTMLElement {
  static tabTitle = "Git";
  static tabOrder = 20;
}

class SandboxConfirm extends HTMLElement {
  static overlay = true;   // 不参与 tab
}

class PlanProgress extends HTMLElement {
  static overlay = true;   // 悬浮窗不参与 tab
}
```

- `tabTitle`：tab 文案；缺省用插件名。
- `tabOrder`：组内排序；缺省 0，再按 Bootstrap 顺序。
- `overlay`：不参与 tab，直接挂在 Slot 下，保持 fixed 定位。

### 3.2 Loader 行为

- v1 只对 `right` Slot 生效：非 `overlay` 的 Mount 自动组成一个 tab 组。
- 所有组件保持挂载并继续接收 State/Command，切换只改 `hidden`。
- tab 条只在组员数大于 1 时显示。
- 当前 tab 存 `localStorage["liteagent.tab.right"]`；tab 被移除时回退到第一个。
- 组件 import 失败只影响自己的 Mount，其他 tab 正常。
- rescan、插件启停后按新 Mount 列表对齐，保留仍存在的组件，不要重新创建它们。

### 3.3 文件与测试

- 修改 `internal/webui/static/loader.mjs`、`internal/webui/static/shell.css`。
- 更新 `internal/webui/loader.test.mjs`：自动分组、overlay 排除、tab 顺序、localStorage、移除后回退、失败隔离。
- 更新 `docs/platform/webui.md` 的 Component Interface 和 Mount 章节。
- `session-trace` 加静态字段；`sandbox-confirm` 加 `overlay`；git 和 plan 组件按第 4、5 节实现。

## 4. git-tools 插件

### 4.1 命名

- 目录：`plugins/git-tools/`。
- 插件名：`git-tools`。
- 能力：`git`。
- UI 包内标签：`git-tools-panel`，Mount ID：`panel`，Slot：`right`，`static tabTitle = "Git"`，`static tabOrder = 20`。
- 现有 [15-git-tools.md](../../design/plugins/15-git-tools.md) 的目录名可复用；方法契约按本文修正。

### 4.2 Manifest

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

`session` 是弱依赖：服务端不直接调用 session，但 UI 用 `session.current` 取 workspace/session_id；缺失时 Git 面板显示空状态。

### 4.3 载荷约定

所有 Git 方法统一接受 ProviderInput 形状，和 file-tools、sandbox seam 一致：

```json
{
  "workspace": "E:/repo",
  "workspace_id": "ws-1",
  "session_id": "s-1",
  "call_id": "call-1",
  "arguments": {}
}
```

- 工具调用时由 tools/tools.call 注入外层字段。
- UI 直接调用时也要包同样形状，便于 sandbox 裁决和审计。
- 路径参数一律相对 workspace；仓库根可以位于 workspace 之上，但路径锚点始终是 workspace。

### 4.4 只读方法

| 方法 | arguments | 工具 | risk |
|---|---|---|---|
| `git.status` | `{}` | `git_status` | 10 |
| `git.diff` | `{path?, staged?}` | `git_diff` | 10 |
| `git.log` | `{limit?}` | `git_log` | 10 |
| `git.branches` | `{}` | 不暴露 | — |

`git.status` 输出至少包含：

```json
{
  "revision": "hash",
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

`revision` 用于 UI 去重；内容不变时不重绘。

### 4.5 写方法

| 方法 | arguments | 工具 | risk |
|---|---|---|---|
| `git.stage` | `{paths: []}` | `git_stage` | 50 |
| `git.unstage` | `{paths: []}` | `git_unstage` | 30 |
| `git.commit` | `{message}` | `git_commit` | 80 |
| `git.checkout` | `{branch}` | `git_checkout` | 80 |

- `git.stage`：路径必须出现在当前 status 中；不支持通配。
- `git.unstage`：支持尚无 HEAD 的初始仓库；无 HEAD 时用 `git rm --cached`，有 HEAD 时用 `git restore --staged` 或 `git reset --`。
- `git.commit`：只提交已暂存内容；message 必填，禁止 `--amend`；提交前检查 staged 是否为空；60 秒超时；返回 commit hash。
- `git.checkout`：只切换已有本地分支，不接受 `create`；工作区有已跟踪改动时拒绝；分支必须来自 `git.branches` 的精确名字。
- 明确不做：discard、reset、rebase、push/pull、创建/删除分支、stash、标签、子模块。

### 4.6 Git 执行约束

- 只使用 `exec.Command("git", args...)`，不拼 shell。
- 用 `git -C <workspace> ...` 启动；先 `rev-parse --show-toplevel` 找仓库根。
- `git status --porcelain=v2 -z --branch --untracked-files=all`；按 NUL 解析，处理 rename/copy 的原始路径、换行文件名。
- `git diff --no-color --no-ext-diff --no-textconv --unified=3`；staged 加 `--cached`。
- `git log` 用 NUL 分隔的格式，限制返回条数。
- 设置 `GIT_OPTIONAL_LOCKS=0`、`GIT_TERMINAL_PROMPT=0`、`GIT_PAGER=cat`；过滤 `GIT_DIR`、`GIT_WORK_TREE`、`GIT_INDEX_FILE`。
- 使用 `--literal-pathspecs` 和 `--`；路径拒绝绝对路径、`..`、NUL、控制字符、Windows 保留设备名。
- 超时：读命令 10–15 秒，commit 60 秒，checkout 30 秒。
- 输出上限：status 约 2 MiB，diff 约 512 KiB，log 约 256 KiB；超限返回 `truncated`。
- 错误码：`git_not_found`、`git_not_repository`、`git_workspace_invalid`、`git_path_invalid`、`git_commit_empty`、`git_worktree_dirty`、`git_conflict`、`git_command_failed`、`git_output_too_large`。

### 4.7 sandbox 配合

sandbox 的 `consumes` 增加：

```json
{"capability": "git", "method": "stage"},
{"capability": "git", "method": "unstage"},
{"capability": "git", "method": "commit"},
{"capability": "git", "method": "checkout"}
```

sandbox 注册对应 seam handler。

- agent 通过 `tools.call` 发起时，外层 `tools.call` 已裁决；内层 git 方法命中 approved 缓存，不重复确认。
- UI 直接调用 git 写方法时，sandbox 通过 `lookupTarget("git", "commit")` 找到工具 risk，按现有风险带裁决。
- sandbox 拒绝/确认超时返回 Tool Result 形状；Git UI 检测 `is_error` 并显示 `content`。
- Git UI 不再加独立模态确认。若 sandbox 未启动，按平台约定直通；提交按钮本身要求 message，切分支在有已跟踪改动时禁用。
- 提交会运行仓库 hooks。UI 必须提示这一点；commit 使用 60 秒超时，不传 `--no-verify`。

### 4.8 UI

右侧 Git tab：

```text
main ↑1 ↓0                    [刷新]
staged 2 · unstaged 1 · untracked 2
▾ 已暂存
  M src/a.go             [取消暂存]
▾ 未暂存
  M src/b.go             [暂存]
▾ 未跟踪
  ? notes.md             [暂存]
▾ 最近提交
  abc123 message
[ 提交说明… ] [提交 2 个文件]
```

- 点击文件打开右侧 diff 抽屉，宽约 760px；显示 hunk、行号；未跟踪文件提示“新文件，暂无 diff”。
- 分支行点击展开本地分支列表（只读列表，不创建/删除）。
- 刷新：tab 激活、窗口 focus、手动按钮、可见时 5 秒轮询；写操作后立即刷新。
- 错误在面板内显示，不额外弹 toast。
- 空状态：无 workspace、非 Git 仓库、找不到 git 命令分别提示。

## 5. plan 插件

### 5.1 命名

- 目录：`plugins/plan/`。
- 插件名：`plan`。
- 能力：`plan`。
- 组件：`plan-progress`，Mount ID `progress`，Slot `right`，`static overlay = true`。
- 本轮不实现 [13-task.md](../../design/plugins/13-task.md) 的完整 task 状态机、依赖图和增量更新；`plan` 是更窄的“长任务计划表”。落地时要在设计稿中标注两者关系，避免两份契约并行。

### 5.2 Manifest

```json
{
  "name": "plan",
  "version": "0.1.0",
  "protocol": 1,
  "entry": "plan",
  "autostart": true,
  "provides": ["plan"],
  "requires": ["session", "tools"],
  "depends_on": ["session", "tools"],
  "ui": {
    "entry": "ui/main.js",
    "components": [
      {"tag": "plan-progress", "mount_id": "progress", "slot": "right"}
    ]
  }
}
```

### 5.3 工具

工具名 `update_plan`，目标方法 `plan.write`，risk 5，`parallel_safe: false`。

```json
{
  "title": "实现 git 与 plan 插件",
  "explanation": "先完成右侧 tab",
  "items": [
    {"step": "实现自动 tab", "status": "completed"},
    {"step": "实现 git 插件", "status": "in_progress"},
    {"step": "实现 plan 悬浮窗", "status": "pending"}
  ]
}
```

- 每次提交完整计划，整体替换旧计划。
- 状态仅 `pending / in_progress / completed`。
- 最多一个 `in_progress`；违反返回 `plan_invalid_transition`。
- 最多 50 项，step 最多 500 字符；空列表拒绝。
- 工具结果返回压缩 checklist、完成数和下一步；不返回整份 JSON。

### 5.4 Session facts

每次写入 `session.append` 一条：

```json
{
  "type": "plan_update",
  "role": "host",
  "content": "Plan updated: 1/3 completed",
  "meta": {
    "title": "...",
    "revision": 3,
    "items": [...],
    "explanation": "..."
  }
}
```

- 在 [session communication](../../../plugins/session/communication.md) 的 Fact 类型表登记 `plan_update`，owner 写 plan。
- 在 session trace 的 `factTypeLabels` 增加 `plan_update: "计划"`。
- plan 插件缓存每个 session 的最新计划；revision 从最新 Fact 继续递增。
- 重启后从 Fact 重建；插件不另存 JSON，不新增构建保留目录。

### 5.5 方法与 session 集成

- `plan.write`：接受 `{session_id, arguments:{title?, explanation?, items}}`；写入 Fact，更新缓存，`SetState("progress", state)` 推送 UI。
- `plan.current`：接受 `{session_id?, arguments:{}}`；显式 session_id 用于 model-context，省略时由 plan 插件调用 `session.current` 解析当前选中会话。
- 新增 `session.current`：返回 `{session_id?, name?, workspace_id, workspace}`。没有选中会话时 session 字段为空，workspace 仍返回当前选择。
- session UI 在选中会话或 workspace 变化时派发：

```js
window.dispatchEvent(new CustomEvent("liteagent:session-changed", {
  detail: { session_id, workspace_id, workspace }
}));
```

- Git 和 plan 都监听该事件；plan 另加 5 秒轮询，覆盖多标签页选中状态不一致。

### 5.6 模型注入

- model-context 在 `prepare` 中读取 `plan.current`，再组装 runtime context。
- 有未完成条目时注入：

```text
## Task plan
1. [completed] 实现自动 tab
2. [in_progress] 实现 git 插件
3. [pending] 实现 plan 悬浮窗
```

- 全部完成或 plan 插件缺席时整节不出现。
- 调用失败静默跳过，不把 model-context 变成 plan 的硬依赖。
- 计划变化会改变 runtime context hash，按现有机制写新的 `runtime_context` Fact。

### 5.7 悬浮窗

- `overlay: true`，`position: fixed`；默认右下，位于输入区上方。
- 可拖拽，可折叠成“计划 2/5”胶囊；位置和折叠状态存 localStorage。
- z-index 低于 sandbox 确认框，避免遮挡确认操作。
- 有活跃计划时出现；当前会话没有计划时隐藏。
- 后台会话的计划更新只缓存不展示；切换会话后重新读取。
- 只读展示，不允许用户直接改状态；模型通过工具更新。

## 6. 跨插件改动清单

| 模块 | 改动 |
|---|---|
| `internal/webui/static/loader.mjs` | 右侧自动 tab、静态元数据、localStorage、overlay |
| `internal/webui/static/shell.css` | tab 条与 panel 样式 |
| `plugins/session` | trace 静态字段、`session.current`、选择事件、`plan_update` 标签 |
| `plugins/sandbox` | consumes 四个 git 写方法；注册 seam handler |
| `plugins/model-context` | plan runtime section；调用 plan.current |
| `plugins/git-tools` | 新插件：能力、工具、UI |
| `plugins/plan` | 新插件：工具、Fact、UI |
| `scripts/shipped-plugins.conf` | core 增加 `git-tools plan` |
| `docs/platform/webui.md` | Component Interface、右侧 tab 行为 |
| `docs/platform/build.md`、`plugins/README.md`、`README*.md` | 插件清单与数量 |
| `docs/design/plugins/15-git-tools.md` | 按本文修正目录、方法、风险、UI、错误码 |
| `docs/design/plugins/13-task.md` | 标注 plan 与 task 的关系；避免两份契约并行 |
| `docs/design/plugins/02-session.md` | `session.current`、`plan_update` 登记 |
| `docs/design/plugins/08-sandbox.md` | git 写方法进入消费清单 |

## 7. 实施顺序

### 阶段 1：右侧自动 tab

- [ ] loader 读取 `tabTitle / tabOrder / overlay`。
- [ ] right Slot 非 overlay Mount 自动分组。
- [ ] 切换只改 hidden；localStorage 记忆；移除后回退。
- [ ] session trace 加静态字段；sandbox 加 `overlay`。
- [ ] loader 测试与 CSS。
- [ ] `webui.md` Component Interface 更新。

验收：原 session trace 正常；sandbox 浮层不参与 tab；刷新后 tab 记忆；插件启停后 Mount 对齐不重建存活组件。

### 阶段 2：git 只读

- [ ] 插件骨架、Manifest、README/communication。
- [ ] git runner：执行、环境清洗、超时、输出上限。
- [ ] status porcelain v2 -z 解析；diff/log/branches。
- [ ] 只读工具注册与 `tools.sync`。
- [ ] Git 面板、workspace 跟随、5 秒轮询、diff 抽屉。
- [ ] 构建清单与文档。

验收：临时仓库 status/diff/log/branches 正确；非仓库、无 git 有明确提示；大 diff 和二进制文件不炸 UI。

### 阶段 3：git 写操作

- [ ] stage/unstage/commit/checkout 后端与校验。
- [ ] 写工具注册，risk 与 `parallel_safe` 正确。
- [ ] sandbox consumes 与 seam handler。
- [ ] UI 按钮、提交框、分支列表、错误显示。
- [ ] 临时仓库测试：暂存、空提交、脏工作区切换、sandbox 拒绝/确认。

验收：agent 与 UI 两条写路径都能被 sandbox 裁决；UI 收到拒绝结果时显示 sandbox content；commit hooks 有提示与超时。

### 阶段 4：plan

- [ ] session 增加 `session.current`、选择事件、`plan_update` 标签。
- [ ] plan 工具、Fact 写入、缓存、revision。
- [ ] `plan.current`、`plan.write`。
- [ ] 悬浮窗、拖拽、折叠、localStorage、会话切换。
- [ ] 构建清单与文档。

验收：update_plan 写入后悬浮窗立即更新；切换会话显示对应计划；后台会话不抢占当前窗口；重启后从 Fact 恢复。

### 阶段 5：模型注入与总验收

- [ ] model-context 读取 plan 并注入 runtime section。
- [ ] 全部完成/缺席/调用失败三种路径。
- [ ] 更新所有设计稿与平台文档。
- [ ] `go test ./...`、`go vet ./...`、`node --test internal/webui/*.test.mjs`、构建验证。
- [ ] 浏览器手工回归：右侧 tab、Git 读写、plan 悬浮窗、压缩后计划注入。

## 8. 测试与验收命令

```bash
go test ./...
go vet ./...
node --test internal/webui/*.test.mjs
bash scripts/build.sh --verify
```

Windows PowerShell 对应：

```powershell
powershell -File scripts\build.ps1 -Verify
```

Git 集成测试需要本机 `git`；测试内部用 `exec.LookPath("git")`，缺失时 `t.Skip`，不要伪造 git 输出替代真实仓库用例。

手工场景：

1. 启动 `dist/liteagent-server`，打开 WebUI。
2. 右侧在“会话 / Git”之间切换，刷新后保持上次 tab。
3. 在仓库里制造 staged/unstaged/untracked 文件，检查分组与 diff。
4. 通过 UI 暂存、提交、切分支；sandbox 启用时检查确认面板。
5. 让 agent 调 `git_status`、`git_diff`、`git_commit`（需要模型配置或直接 bridge 调用）。
6. 调 `update_plan`，检查悬浮窗、Trace 中的 `plan_update`、切换会话后的显示。
7. 触发一次上下文压缩，确认下一轮 runtime context 仍包含 `Task plan`。

## 9. 与现有设计稿的差异

### 9.1 `15-git-tools.md`

现有稿与本次共识的差异：

| 项 | 现有稿 | 本次决定 |
|---|---|---|
| UI | 无 | 右侧 Git 侧边栏 + diff 抽屉 |
| 插件目录 | `plugins/git-tools/` | 复用 |
| 方法 | commit 同时暂存路径；checkout 可 create | 拆分 stage/unstage；commit 只提交已暂存；checkout 只切已有分支 |
| 工具 | 5 个 | 7 个（增加 stage/unstage） |
| 写风险 | 60 | stage 50、unstage 30、commit/checkout 80 |
| sandbox | 待定 | 明确消费四个写方法 |
| 大 diff | 落盘 artifacts | v1 只截断，不落盘（如后续需要再加） |

落地时更新该设计稿，或标注被本文取代；不要留下两份冲突契约。

### 9.2 `13-task.md`

现有稿是完整的 `task` 插件：增量条目、依赖图、状态机、4 个状态、清空与查询。本轮 `plan` 只做：

- 单会话一份计划；
- 全量替换；
- 3 个状态；
- 一个 `update_plan` 工具；
- 悬浮窗进度展示。

两者不是同一实现。落地时在 `13-task.md` 顶部标注“本轮未采用；由 plan 插件覆盖长任务进度场景”，避免后续误读。

### 9.3 sandbox 现状

当前 `plugins/sandbox/plugin.json` 的 `autostart` 是 `true`，但设计稿和部分文档把它写成可选件 `autostart:false`。本轮以 Manifest 为准；如果发布配置要把它改成可选，Git 写操作在 sandbox 缺席时会直通，这是既定降级行为，不额外加插件内确认。

## 10. 风险与开放点

1. **自动 tab 是 Component Interface 变化。** 静态属性必须在组件类定义后读取；Loader 已等待 `customElements.whenDefined`，实现时不要在 `connectedCallback` 前读实例字段。
2. **隐藏组件仍接收推送。** 这是有意设计，保证切回 tab 时状态最新；不要改成懒挂载，否则 session trace 的增量游标会断。
3. **Git UI 直接写调用会经过 sandbox。** sandbox 拒绝时返回的是 Tool Result 形状，不是正常 `{commit}` / `{branch}`，UI 必须兼容。
4. **commit hooks 与签名。** 提交可能运行仓库 hooks，也可能因 GPG 签名等待输入；必须设置超时，并在 UI 提示。不要传 `--no-verify`。
5. **Git 路径锚点是 workspace。** 仓库根可能在工作区之上；status/diff 输出要按 workspace 相对路径处理，避免把仓库外的变更带进面板。
6. **plan 注入依赖 plan 插件活跃。** 调用失败必须静默跳过；不能在 model-context 里把 plan 变成硬依赖或阻断 Turn。
7. **plan 悬浮窗多标签页。** 选择事件只在触发页面派发；5 秒轮询是兜底，不要去掉。
8. **现有未提交修改。** 见第 2.4 节；实现前先确认这些改动属于用户当前工作，不要 `git checkout` / `git reset` / `git clean`。
9. **文档冲突。** `15-git-tools.md` 和 `13-task.md` 必须同步处理；否则后续会话会按旧契约实现。

## 11. 关键文件地图

| 内容 | 位置 |
|---|---|
| 右侧 Slot | `internal/webui/static/index.html` |
| Mount Loader | `internal/webui/static/loader.mjs` |
| Shell 样式 | `internal/webui/static/shell.css` |
| Mount Bootstrap | `internal/webui/bootstrap.go`、`internal/webui/registry.go` |
| Session 方法注册 | `plugins/session/main.go` |
| Session UI | `plugins/session/ui/main.js` |
| Sandbox seam | `plugins/sandbox/seam.go`、`plugins/sandbox/main.go`、`plugins/sandbox/plugin.json` |
| model-context 组装 | `plugins/model-context/assemble.go`、`plugins/model-context/methods.go` |
| 工具注册契约 | `plugins/tools/communication.md` |
| 构建清单 | `scripts/shipped-plugins.conf`、`scripts/build.sh`、`scripts/build.ps1` |
| Git 设计稿 | `docs/design/plugins/15-git-tools.md` |
| Task 设计稿 | `docs/design/plugins/13-task.md` |
| WebUI 平台契约 | `docs/platform/webui.md` |
| Manifest 契约 | `docs/platform/manifest.md` |

## 12. 下一步

从阶段 1 开始，先只改 Loader 和测试：

1. 给 `session-trace` 加 `static tabTitle = "会话"`、`static tabOrder = 10`。
2. 给 `sandbox-confirm` 加 `static overlay = true`。
3. 在 `loader.mjs` 实现 right Slot 自动分组与 localStorage。
4. 补 `loader.test.mjs` 用例并跑 `node --test internal/webui/*.test.mjs`。
5. 确认现有 session 布局无回归后，再开始 git-tools 后端。
