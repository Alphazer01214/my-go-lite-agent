# model-context 与工具链实现交接

> 状态：当前实现
> 说明：本文的插件清单与 `scripts/shipped-plugins.conf` 一致，是当前插件状态的最新记录。

本文记录 `plugins` 分支上 model-context、tools、file-tools 和 shell-tools 的实现状态。设计输入见 [消费链组合方案](../../design/composition-model-context-tools-sandbox.md)。

## 1. 交付范围

| 插件 | 状态 | 内容 |
|---|---|---|
| `model-context` | 已实现 | `llm.complete` 消费者；system / runtime Facts；紧凑快照；确定性压缩 |
| `tools` | 已实现 | `sync` / `register` / `unregister` / `list` / `call`；Generation 可用性门；注册表缓存 |
| `file-tools` | 已实现 | 读写八件套 |
| `shell-tools` | 已实现 | `shell_execute`；同步执行；进程树取消 |
| `agent` | 已适配 | 基线构建；`llm.complete` 携带 `meta`；工具调用传递 workspace/mode |
| `session` | 已适配 | system 置顶、runtime context、shadow range 压缩、`seq` / `meta` 输出 |

Web 工具、sandbox、subagent 不在本轮。

## 2. 当前仓库状态

- 分支：`plugins`。
- 最近提交：`93f0e45 doc`。
- 本轮改动尚未提交。
- 新增目录处于 untracked：`plugins/model-context/`、`plugins/tools/`、`plugins/file-tools/`、`plugins/shell-tools/`（各插件使用本地 `wire.go`）。
- `go.mod` 新增 `golang.org/x/sys v0.34.0`，用于 Windows Job Object。
- `plugins/tools/registry.json` 是运行时缓存，已加入 `.gitignore`。
- 部分 agent/session 文件只出现 LF/CRLF 行尾差异，`git diff` 内容为空，不要误删。

## 3. 核心不变量

### 3.1 Session

- Session 是唯一真源，只追加。
- 模型可见内容必须已记录为 Fact。
- `session.derive` 是纯投影；日志顺序不等于模型顺序。

投影顺序：

```text
最新 system prompt
→ 最新 active 摘要（shadow range）
→ 最新 runtime context
→ 普通消息（不在遮蔽区间）
→ tool result stub
```

- system prompt 与 runtime context 不受摘要遮蔽影响。
- 压缩只写 `context_summary` 与 `context_compaction`，不改写旧消息。
- `context_snapshot.surface` 记录实际模型顺序的 Seq 列表。

### 3.2 model-context

- 消费 `llm.complete`，priority 2000。
- 先写 system / runtime Facts，再 `session.derive`，最后 `Proceed`。
- `context_snapshot` 在调用模型前写入；写入失败则不调用模型。
- `request_header` 是 agent 基线摘要；快照是权威请求记录。
- 自动压缩只做确定性摘要；`inspect` 只覆盖当前进程。

### 3.3 tools

- tools 只是 `tools.call` 的 Provider，不是 `file.*` / `shell.*` 的 Owner。
- 分发必须走 Host Call；否则 sandbox 等消费者会被绕过。
- Provider 启动时 `tools.sync` 原子替换本来源全集。
- 注册要求来源 Manifest `provides` 包含目标 Capability。
- list/call 要求来源 active、目标方法已注册、Generation 与注册时一致。
- 注册表缓存到 `registry.json`；Provider 重启后仍须重新 `sync`。
- `SyncAll` 会持续重试，直到 tools 就绪。

### 3.4 Risk

`risk` 是 0–100 整数，越高越危险：

| 范围 | 含义 |
|---:|---|
| 0–19 | 只读或无副作用 |
| 20–49 | 可逆本地修改 |
| 50–79 | 显著本地修改 |
| 80–100 | 任意代码执行或破坏性操作 |

当前取值：

- 只读文件工具：`10`。
- 文件写入、替换、补丁：`80`。
- 命令执行：`90`。

后续 sandbox 按阈值过滤，类似 priority。

## 4. 关键文件

| 内容 | 文件 |
|---|---|
| model-context 契约 | [plugins/model-context/communication.md](../../../plugins/model-context/communication.md) |
| model-context 实现 | `plugins/model-context/builder.go`、`prompt.go`、`main.go` |
| tools 契约 | [plugins/tools/communication.md](../../../plugins/tools/communication.md) |
| tools 实现 | `plugins/tools/main.go`、`registry.go`、`host.go` |
| tool JSON 契约 | [plugins/tools/communication.md](../../../plugins/tools/communication.md)（各插件本地 `wire.go`） |
| file-tools 契约 | [plugins/file-tools/communication.md](../../../plugins/file-tools/communication.md) |
| shell-tools 契约 | [plugins/shell-tools/communication.md](../../../plugins/shell-tools/communication.md) |
| Session 契约 | [plugins/session/communication.md](../../../plugins/session/communication.md) |
| Session 投影 | `plugins/session/derive.go` |
| agent 契约 | [plugins/agent/communication.md](../../../plugins/agent/communication.md) |
| agent 基线 | `plugins/agent/turn.go` |
| 构建清单 | `scripts/shipped-plugins.conf` |
| 构建说明 | [docs/platform/build.md](../../platform/build.md) |

## 5. 验证记录

已执行：

```text
go test ./...
go vet ./...
go test -race ./plugins/tools/ ./plugins/file-tools/ ./plugins/shell-tools/ ./plugins/agent/ ./plugins/model-context/
```

结果全部通过。

端到端验证使用本地假 OpenAI 服务，不访问真实上游。流程：

1. 假 LLM 首次返回 `file_read` 工具调用。
2. agent 调用 `tools.call`。
3. tools 分发到 `file.read`，file-tools 读取 README。
4. tool 结果写入 Session。
5. 第二步假 LLM 收到 tool 消息并返回最终正文。

实测结果：

```text
tools: file_glob(risk=10), file_list(risk=10), file_patch(risk=80),
file_read(risk=10), file_replace(risk=80), file_search(risk=10),
file_stat(risk=10), file_write(risk=80), shell_execute(risk=90)

turn: steps=2, reply="done: tool result received"
tool result: "# LiteAgent ..."
snapshot tools: 9 个工具全部存在
```

复现时需要的环境变量：

```text
OPENAI_BASE_URL=http://127.0.0.1:<fake-port>/v1
OPENAI_API_KEY=test
OPENAI_MODEL=fake
LITEAGENT_HTTP_ADDR=127.0.0.1:<web-port>
SESSION_DATA_DIR=<tmp>/sessions
TOOLS_REGISTRY_FILE=<tmp>/tools-registry.json
```

临时假服务和会话数据已删除，未进入仓库。

## 6. 已知边界

- Host fail-open 未实现：model-context Manifest 存在但进程未激活时，`llm.complete` 仍返回 `route_failed`。
- `consumes[].priority` 未实现；当前使用插件级 priority。
- sandbox 未实现；`file.write` 与 `shell.execute` 目前只有 Provider 自身约束。
- agent 模式过滤未实现；`tags` 已进入 Tool Definition 与 `toolSpec`。
- Web 工具、`file_move`、`file_delete`、后台 Shell、PTY 未实现。
- model-context 只做确定性压缩，不做 LLM 摘要。
- `model-context.inspect` 只查当前进程；持久记录在 `context_snapshot` Fact 中。
- tools 注册表按插件名归属；同一 Provider 多实例共享一份，最后一次 `sync` 生效。
- Shell 默认拒绝通用解释器；Windows 依赖 `x/sys` Job Object。
- 端到端验证覆盖 `file_read`；未覆盖 `file_write`、`shell_execute` 与 sandbox 拒绝路径。

## 7. 下一步建议

1. 实现 sandbox：按 risk 阈值做 allow/deny，消费 `tools.call` 与 Provider 方法。
2. 补 Host fail-open（P1）；按需实现按条目 priority（P2）。
3. 实现 agent 模式：`modes` 配置、tags/names 过滤、模式记录。
4. 第二批工具：`file_move`、`file_delete`、Web Fetch/Search、后台 Shell。
5. 评估 session 进入 `llm.complete` / `tools.call` 链，统一记录请求与工具调用。
6. 提交本轮改动并跑一次完整构建与插件中心回归。

## 8. 交接检查清单

- [ ] 确认是否先提交本轮改动。
- [ ] 重新运行 `go test ./...` 与 `go vet ./...`。
- [ ] 跑一次 `scripts/build.sh` 或 `scripts/build.ps1`，确认新增插件进入 dist。
- [ ] 确认 `model-context`、`shell-tools` 配置种子与 `registry.json` 缓存路径。
- [ ] 与用户确认 sandbox 的 risk 阈值与 deny/confirm 返回形态。
