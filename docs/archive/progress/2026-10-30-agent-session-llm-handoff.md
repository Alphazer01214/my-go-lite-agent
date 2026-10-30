# Agent→Session→LLM 基础链路与 Session WebUI 后续实现（2026-10-30）

> 状态：待实现（当前基线已实现；本文列出剩余实现、接入点与验收）
> 关联：[plugins/agent](../../../plugins/agent/)、[plugins/session](../../../plugins/session/)、[plugins/llm-openai](../../../plugins/llm-openai/)、[plugins/session/ui](../../../plugins/session/ui/)、[scripts/build.ps1](../../../scripts/build.ps1)、[scripts/build.sh](../../../scripts/build.sh)

## 0. 当前基线

已经跑通一轮完整对话：

1. 浏览器调 `loop.turn`；
2. agent `session.info`，不存在则 `session.create`（用首条输入生成标题）；
3. agent `session.append` 写 `turn_start`；
4. 写用户 `message`；
5. `session.messages` 投影历史（只取 `type=message`）；
6. `llm.stream`，agent 用 callback 转发 `llm.chunk` 为 `loop.chunk`；
7. agent 把模型返回的 assistant message 写回 session；
8. 写 `turn_end`，返回 `{session_id, messages, seqs}`。

已实现的方法与事件：

| 方法/事件 | 契约 |
|---|---|
| `session.update` | `{session_id,title?}` / `{session_id,pinned?}`，追加 meta 修订行并刷新索引 |
| `session.delete` | `{session_id}`，关闭句柄、移除索引、删除 JSONL |
| `loop.cancel` | `{session_id}` → `{session_id,cancelled}`；取消该会话的活动 Turn |
| `loop.started` | `evt {session_id,turn_id}`，在会话建立后、`turn_start` 前发出 |
| `turn_start` Fact | `role=host`，`meta={turn_id,workspace}` |
| `turn_end` Fact | `role=host`，`meta={turn_id,status,duration_ms,error?}`，status 为 `ok/error/cancelled` |

阶段约束：

- 不使用任何 `InContext` 变体，跨插件调用只走默认实例的 `Call` / `CallWithCallback` / `EmitWithID`；
- Session 只追加；`type=message` 是模型可见投影的唯一入口；
- chunk 事件是尽力而为的进度信号，最终 `res` 才是真源；
- agent 同一会话同时只允许一个 Turn，冲突返回 `turn_busy`。

当前 UI：`session-history`（left）、`session-chat`（center）、`session-trace`（right），已支持流式、停止、重命名、置顶、删除、facts 消息/非消息筛选。

## 1. 启动与构建

构建脚本已做通用化：

- 每个插件根目录的运行时 `*.json`（`plugin.json` / `config.example.json` / `segments.json` 除外）以及 `sessions/`、`data/` 目录都会在 clean build 前自动暂存、构建后恢复；
- 新插件如果只有 `config.example.json`，构建会从仓库本地 `config.json` 或示例自动播种缺失的 `config.json`；
- 构建时会校验 manifest 的 `ui.entry` 是否存在（Unix 侧在 `python3`/`node` 可用时执行），缺文件只告警不中止；
- `scripts/build.sh` 与 `scripts/build.ps1` 行为保持一致，差异只在平台二进制与编码处理。

命令：

```powershell
# Windows
powershell -File scripts\build.ps1 -Clean -Verify
.\dist\liteagent-server.exe
```

```bash
# macOS / Linux
bash scripts/build.sh --clean --verify
./dist/liteagent-server
```

启动后访问 `http://127.0.0.1:2333/`。

验证：

```bash
go build ./...
go vet ./...
go test ./...
```

新增插件的检查清单：

1. `plugins/<name>/plugin.json`，含 `entry`、`provides`、必要的 `requires`/`depends_on`、`ui`；
2. `ui/<entry>` 与 custom elements；`ui.entry` 必须在插件包内；
3. 有运行时配置时提供 `config.example.json`；
4. 把插件名加入 `scripts/shipped-plugins.conf`；
5. 补 `plugins/<name>/` 文档和测试。

已知缺口：

- `model-context`、`tools`、`file-tools`、`shell-tools`、`subagent`、`sandbox` 目前只有 Go 骨架，**源码没有 `plugin.json`**。dist 里这些目录是重构前留下的旧拷贝，clean build 不会重新生成它们；不要把 dist 里的行为当作当前实现。
- `plugins/llm-openai/config.json`（gitignored）在本机仍是旧扁平格式；dist 里已是 `models[] + default_model`。构建脚本会对此告警；迁移方向见 2.3。

## 2. 实现顺序

### 2.1 LLM tool calls 与多步 Turn

目标：模型可以请求工具，agent 执行后把 Tool Result 写回 session，再继续下一 Step。

llm-openai 侧：

- `readSSE` 需要累计 `delta.tool_calls` 分片：按 `index` 拼接 `id`、`function.name` 和 `function.arguments`；
- `streamResp.Messages` 允许返回 assistant 的 `tool_calls`；
- 请求体增加 `tools[]`（`pluginsdk.Tool`），由调用方传入。

agent 侧：

- 从 `tools.list` 拿工具表，随 `llm.stream` 一起传；
- 模型返回 tool_calls 时，逐个 `tools.call`，把 Tool Result 作为 `role=tool` 的 `message` Fact 追加；
- 循环直到模型返回纯文本或达到 `max_steps`；
- 每步追加 `step_start` / `step_end` Fact（`meta={step,turn_id,tool_calls?}`），便于右侧 trace；
- 子代理工具沿用同一分发方式，不新增并行执行器。

验收：

- 单工具、多工具、连续两轮工具调用都能在 UI 的消息与 facts 中看到；
- 工具失败作为 Tool Result 返回模型，不直接终止 Turn；
- `max_steps` 截断时写 `turn_end(status=error)` 或明确的 `max_steps` meta。

### 2.2 model-context 接入

当前 agent 直接 `session.messages` → `llm.stream`，没有 system prompt、runtime context 或压缩。

需要先定链的位置，再写代码：

- 方案 A：model-context 声明 `consumes: llm.stream`，在 `Proceed` 前读 session、写 prompt/context Facts，再让 agent 的请求继续到 llm-openai；
- 方案 B：agent 在调用 llm 前显式 `model-context.build`，把结果并入 messages。

无论选哪个，都必须遵守：

- system prompt、runtime context、压缩摘要先写 Fact，再投影给模型；
- `session.messages` 目前只返回 `type=message` 且 role 非空的 Fact；system prompt 需要单独排序到头部，或由 model-context 提供独立的投影接口；
- 静态 prompt、skills、MCP 的正文不能由插件直接拼进 system prompt，owner 仍是 model-context。

验收：

- 同一会话连续两轮，模型收到的 system prompt 字节稳定；
- 刷新后能从 Facts 复原当时发送的 messages；
- 压缩只追加 Fact，不删除旧消息。

### 2.3 Settings 方法

`agent` 与 `llm-openai` 的 manifest 已声明 `settings.get/save`，但 handler 未注册，插件中心的设置页会报 `method_not_found`。

- `agent.settings.get/save`：读写 `default_scheme`、`max_steps`；
- `llm.settings.get/save`：读写 `models[]`、`default_model`，API Key 三态（不传保留、空串清除、新值覆盖）；
- 保存后立即生效并落盘 `config.json`；失败返回可读的 `handler_error`；
- 同时把仓库本地的旧扁平 LLM 配置迁移为 `models[] + default_model`，保留原 API Key；
- 前端不需要改，`settings.mjs` 已按 schema 渲染。

验收：插件中心保存后重启 server，配置仍然生效；`config.get`（如果实现）不回显明文 key。

### 2.4 UI 后续

优先级从高到低：

1. **消息与 facts 分页**：`session.facts` 已有 `after_seq/limit`；`session.messages` 需要同样参数或单独的 `session.state` 快照。长会话当前会一次性读全文件。
2. **多标签/后台实时更新**：当前 UI 只在自己发起调用或重连 `resync` 时刷新。可让 session 在 append 后 `ui.state` 推送聊天快照，或新增订阅方法。
3. **取消时部分文本落库**：现在取消只保留用户 Fact 和 `turn_end(cancelled)`，UI 的草稿在刷新后消失。若要保留，写一条带 `meta.cancelled=true` 的 assistant message 或独立 Fact。
4. **workspace 重命名/删除**：后端已有 `workspace.update/delete`，左侧只做了选择与添加。
5. **模型/用量展示**：session Meta 目前没有 model/context_window；需要 `session.create` 扩展字段并在中间头部或底部展示。
6. **附件/图片消息**：`ChatMessage.Files` 已存在，UI 和 llm-openai 都还未处理。
7. **Markdown 流式渲染**：当前流式阶段是纯文本，结束后才按 Markdown 重建；可选做增量渲染。

### 2.5 Turn 与删除的跨标签协调

`session.delete` 不知道 agent 是否有活动 Turn。UI 只对本地已知的运行中会话禁用删除。

建议：

- agent 提供 `loop.active {session_id}` 或 `loop.active` 列表；
- 或在 session 侧维护一个由 agent 写入的轻量占用标记；
- 删除运行中的会话时返回 `turn_busy`，由 UI 提示。

### 2.6 用量与可观测性

- llm-openai 解析上游 `usage`（如果供应商返回），写入 `llm_usage` Fact；
- agent 在 `turn_end` 之外记录 `request_header`（model、tools、message hash）和实际 step 数；
- 右侧 trace 已经能展示任意非 message Fact，不需要改 UI。

## 3. 不变量（实现时必须保持）

1. 模型可见的内容必须先以 Fact 记录；流式 delta 不落库，最终 message 落库。
2. `type=message` + role 是模型消息的判定；生命周期 Fact 不得进入 `session.messages`。
3. `loop.chunk` / `loop.started` 是尽力而为事件；最终 `res` 或 session Fact 才是结果真源。
4. 不使用 `InContext`、多实例绑定和 `context_id`，直到明确需要。
5. 取消使用独立 `turnCtx`；`turn_end` 用 detached 短 ctx 落库。
6. Session 文件只追加；meta 用修订行，删除才移除文件。

## 4. 文件索引

| 位置 | 当前职责 |
|---|---|
| `plugins/agent/functions.go` | Turn 编排、session 调用、llm 调用、取消注册、turn facts |
| `plugins/agent/plugin.go` | activeTurn 注册表 |
| `plugins/session/functions.go` | list/info/append/facts/messages/update/delete |
| `plugins/session/store.go` | JSONL、meta 修订、快照 |
| `plugins/session/ui/store.js` | UI 状态、所有桥调用、Turn 生命周期 |
| `plugins/session/ui/chat.js` | 消息、流式草稿、发送/停止 |
| `plugins/session/ui/trace.js` | Facts 分类与展开 |
| `plugins/llm-openai/functions.go` | SSE 解析、chunk 事件、assistant message 组装 |
| `scripts/build.ps1` / `build.sh` | 构建、runtime 数据保留、config 播种、UI entry 校验 |

## 5. 验证清单

- `go build ./...`、`go vet ./...`、`go test ./...`；
- clean build 后检查：每个插件 manifest 的 `ui.entry` 存在；`dist/plugins/<name>/config.json` 与 `sessions/` 未被清空；
- 浏览器：新建会话、流式、停止、重命名、置顶、删除、非消息 facts、分栏拖拽；
- `loop.turn` 的 `loop.started` 必须是第一条事件；取消后原 turn 返回 `call_cancelled`；
- 取消后的会话 facts 为 `turn_start → user message → turn_end(cancelled)`，没有 assistant。
