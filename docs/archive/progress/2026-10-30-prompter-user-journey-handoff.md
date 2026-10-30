# Prompter 用户路径回归与问题清单（2026-10-30）

> 状态：仅参考（用户视角执行证据与建议方案；修复前先确认产品边界）
> 关联：[plugins/prompter](../../../plugins/prompter/)、[plugins/session](../../../plugins/session/)、[plugins/agent](../../../plugins/agent/)、[plugins/llm-openai](../../../plugins/llm-openai/)、[plugins/session/ui](../../../plugins/session/ui/)、[scripts/build.ps1](../../../scripts/build.ps1)、[scripts/build.sh](../../../scripts/build.sh)

## 0. 本轮范围

本轮在 Windows 上用 `scripts/build.ps1` 构建 `dist/`，启动 `liteagent-server.exe`，以用户路径走完以下场景：

- 新建会话；
- 连续两轮追问；
- 长文生成中取消；
- 生成中刷新页面；
- 选择不可用模型；
- 插件中心停止/启动 prompter；
- 请求读取工作区文件；
- 会话列表、facts 面板、诊断面板、删除会话。

模型使用 `mimo-v2.6-flash`；工作区为仓库根目录。测试创建的 4 个会话已通过 `session.delete` 清理，既有会话未动。测试服务已停止。

## 1. 已实现的基线

上一轮已把 prompter 作为消费者接入：

| 位置 | 当前行为 |
|---|---|
| `session.create` | 接收可选 `facts`；建日志后、注册前按序写入；失败删文件回滚 |
| prompter `session.create` | 补 `system_prompt`、`runtime_context` 两条 fact 后 `Proceed` |
| prompter `llm.stream` | 读 `session.info` 与 `session.facts`；按文本差异补基线；写 `context_snapshot`；投影 messages 后 `Proceed` |
| 投影顺序 | 最新 `system_prompt` → 最新 `runtime_context` → `message` facts 按 seq 升序 |
| 失败行为 | prompter 缺席或装配失败时按原 payload 透传 |
| agent | `llm.stream` 带 `meta.session_id/turn_id/workspace/model`；保留 `messages` 作为兜底 |

已验证：

- 新会话 facts 为 `system_prompt → runtime_context → turn_start → user → context_snapshot → assistant → turn_end`；
- 第二轮 snapshot 的 `surface_seqs` 包含历史消息，`runtime_context` 不重复写；
- 停止 prompter 后新会话仍可对话；重启后旧会话下一轮自动补基线；
- `go test ./...` 全部通过。

## 2. 问题清单

### P0-1 工具链缺失，模型输出伪 tool_call

**复现。** 新会话选择 `mimo-v2.6-flash`，发送“读取工作区 README.md 的前 5 行，并原样返回。”。

**现象。** assistant 返回原始文本：

```text
<tool_call><function=Read><parameter=file_path>E:\Projects\my-go-lite-agent\README.md</parameter></function></tool_call>
```

没有工具执行，没有工具结果，没有错误提示；`turn_end` 仍是 `status=ok`。上一轮问“你能做什么”时，模型还声称可以“读写代码、运行命令”。

**原因。** agent 的 Turn 只调用一次 `llm.stream`，没有 `tools` 字段、没有 Step 循环、没有工具结果回填；当前 prompt 又把自己描述成可读写工作区的 agent。模型于是按训练分布自行编造工具调用。

**建议方案。**

1. 先定产品边界：做工具型 agent，还是当前只做纯对话。
2. 工具型：按 `2026-10-30-agent-session-llm-handoff.md` 2.1 实现 `llm-openai` 的 `tools` 与 `tool_calls` 流式累积、agent 多 Step 循环、`tools.call`、tool result fact、`max_steps` 与 `step_start/step_end`。
3. 纯对话：修改默认 system prompt，明确“当前没有工具，不能读取文件或执行命令”，并在模型请求文件操作时直接说明限制；不要保留暗示工具能力的身份句。
4. 无论哪条路，都要让 prompt 中的工具纪律随真实工具表条件渲染，不能描述不存在的工具。

**验收。** 同一请求要么返回真实 README 内容，要么给出明确的能力限制；assistant 正文中不再出现原始 `<tool_call>`。

**落点。** `plugins/agent/functions.go`、`plugins/llm-openai/functions.go`、`plugins/prompter/config.example.json`、工具插件 manifest。

### P0-2 刷新后不恢复进行中的 Turn

**复现。** 发送长文请求，生成中刷新页面，等待后台完成，再查看当前会话。

**现象。** 后端继续流式并最终写入 assistant；新页面一直停在刷新前的消息数和 facts 数。手动切换到其他会话再切回来，才出现完整回答。

**证据。** 后端最终 `26 facts / 9 messages`；页面仍是 `24 facts / 8 messages`；切换会话后变为 9 条消息。服务端日志在刷新后仍持续输出 `llm-openai` chunk。

**原因。** 刷新只重连 `/bridge/ui`，UI 不知道当前会话还有活动 Turn；旧的 HTTP 调用已经断开，新页面没有订阅或轮询进行中的 Turn。

**建议方案。**

1. agent 提供 `loop.active {session_id}` 或在 session 维护活动 Turn 标记，UI 重连后查询并恢复“生成中”状态。
2. session 在 assistant / `turn_end` 落库后发 `ui.state` 或触发当前会话刷新。
3. UI 重连时主动拉取当前会话快照；若发现 `turn_start` 没有对应 `turn_end`，显示后台生成中并轮询直到完成。
4. 完成后给一次提示，避免用户误以为消息丢失。

**验收。** 生成中刷新，后台完成后当前会话自动出现 assistant，facts 计数与服务端一致，无需手动切换会话。

**落点。** `plugins/session/ui/store.js`、`plugins/session/ui/chat.js`、`internal/webui/publisher.go`、agent 的 Turn 状态接口。

### P1-1 取消后未完成的 user message 留在上下文

**复现。** 发送长文请求，生成中点击停止；随后发送“忽略上一轮未完成的文章。只回答：1+1=？”。取消轮 facts 只有 user message，没有 assistant，`turn_end(status=cancelled)`。

**现象。** 被取消的 user message 仍是 `type=message`，prompter 在下一轮会照常投影。模型能看到一条没有回答的用户请求；用户必须手动说“忽略”才不会被带偏。UI 会显示“已停止 · 本轮内容未保存”，但下一轮切换会话后部分文本消失，只留下 user message。

**建议方案。**

1. 取消时给该 user message 写 `meta.cancelled=true`，或追加一条 `turn_cancelled` fact；prompter 投影时跳过没有 assistant 的 cancelled turn。
2. 或者在下一轮请求前插入系统说明：“上一条用户请求已被用户取消，不要继续执行。”
3. 提供“重试上一条”入口，重试时复用原 user message。

**验收。** 取消后不显式说“忽略”也不会把旧请求带入下一轮；重试仍能得到回答。

**落点。** `plugins/agent/functions.go`（取消路径）、`plugins/prompter/context.go`（投影）、`plugins/session/ui/chat.js`。

### P1-2 模型端点不可用时会话变成死会话

**复现。** 新会话选择 `qwen2.5-0.5b`（本机端点未启动），发送任意消息。

**现象。** 会话已经按 qwen 创建；`llm.stream` 失败后模型选择器对该会话禁用，无法在会话内切回可用模型。用户只能新建会话，并留下一个没有 assistant 的失败会话。

**建议方案。**

1. 创建会话前做模型健康检查，或至少在首轮失败后允许修改模型。
2. 为“尚无成功 assistant”的会话增加 `session.update_model` 或 agent 重试参数，允许换模型重试。
3. UI 在失败状态提供“换模型重试”和“删除此会话”。

**验收。** 端点不可用时用户能在同一会话内换模型并完成回答；失败会话不会成为死路。

**落点。** `plugins/agent/functions.go`、`plugins/session/functions.go`、`plugins/session/ui/store.js`、`plugins/llm-openai/functions.go`。

### P1-3 错误展示重复且归因错误

**复现。** 选择不可用的 `qwen2.5-0.5b` 发送消息。

**现象。** 同一错误出现三次：顶部 banner、聊天区红字、左下 toast。诊断面板把它归到 `loop.turn` 与 `handler_error`，内容是：

```text
Post "http://localhost:1145/chat/completions": dial tcp [::1]:1145: connectex: No connection could be made because the target machine actively refused it.
```

用户看不出是 llm-openai、哪个模型、下一步该做什么。

**建议方案。**

1. llm-openai 返回稳定错误码，例如 `llm_connection_failed`、`llm_auth_failed`、`llm_timeout`、`llm_model_unavailable`，并附带模型 ID 与可读建议。
2. agent 只做一次错误映射，不再叠加 toast/banner/内联三份。
3. WebUI 诊断保留来源插件、capability、method，不要把下游错误归到 `loop.turn`。
4. 诊断消息面向用户，不直接展示 Go 网络错误原文。

**验收。** 一次错误只出现一次；诊断能定位到 `llm-openai / llm.stream`；文案包含“启动本地模型服务”或“切换模型”等动作。

**落点。** `plugins/llm-openai/functions.go`、`plugins/agent/functions.go`、`internal/webui/bridge.go`、`internal/webui/management.go`。

### P1-4 插件中心不展示 consumes 链

**复现。** 打开插件中心，查看 prompter 节点与关系列表。

**现象。** 图上只有“装载”和“硬依赖”，没有 `consumes` 边；prompter 详情里的 registration 能看到 `llm.stream`、`session.create`，但看不到它是消费者、链序和 route owner。关系列表 8 条也不含消费关系。

**建议方案。**

1. 插件快照加入 manifest 的 `consumes` 与 priority。
2. 图中增加“消费”边，标注 `capability.method` 和 priority。
3. 详情区分 Provider 与 Consumer 角色，显示当前 route owner。

**验收。** 图中能看出 prompter 消费 `session.create` 与 `llm.stream`，并知道它位于 session/llm-openai 之前。

**落点。** `internal/webui/plugin_snapshot.go`、插件中心前端、`internal/plugin/manifest.go`（如果快照未包含 consumes）。

### P1-5 Prompter 没有配置入口

**复现。** 插件中心选中 prompter，设置区显示“该插件未提供可配置项。”

**现象。** 用户只能手动编辑 `dist/plugins/prompter/config.json` 并重启插件；prompt sections、skills、MCP 都不可见。

**建议方案。**

1. prompter manifest 增加 `settings.get/save` 与 schema，至少覆盖 `prompt_sections`、`skills`、`mcp_servers`。
2. 保存时校验 section 名、track、变量、skills/MCP 重名；非法配置不落盘。
3. 提供 reload 或说明“下一次请求生效”，避免用户必须重启整个进程。

**验收。** 用户能在插件中心看到并修改至少一个 prompt section；保存后下一轮 facts 中的 `system_prompt` 哈希变化。

**落点。** `plugins/prompter/plugin.json`、`plugins/prompter/config.go`、`plugins/prompter/README.md`、插件设置渲染。

### P1-6 构建裁剪了工具插件目录

**复现。** 在源码状态运行 `scripts/build.ps1`。

**现象。** `dist/plugins` 只剩 `agent / llm-openai / prompter / session`；`tools / file-tools / shell-tools / sandbox / subagent` 的旧产物被 prune。原因是这些插件源码没有 `plugin.json`，构建跳过安装，随后清理了 dist 中未重建的目录。

**影响。** 插件中心只有 4 个插件，agent 没有工具可接；与 P0-1 相互放大。

**建议方案。**

1. 为这些插件补 `plugin.json`（从旧产物或注册代码推导），或者明确它们不属于当前发行版并从 `shipped-plugins.conf` 移除。
2. 调整构建：对“源码存在但没有 manifest”的插件，至少保留 dist 旧目录或给出显式错误，不静默 prune。
3. 在构建输出中列出被 prune 的插件名与原因。

**验收。** 构建后 `dist/plugins` 与 `shipped-plugins.conf` 一致；插件中心能看到预期插件；agent 工具链可拿到工具表。

**落点。** `scripts/build.ps1`、`scripts/build.sh`、`scripts/shipped-plugins.conf`、工具插件目录。

### P2-1 首轮和长文延迟高

首轮 `turn_end.duration_ms` 约 41.6 秒；文件读取请求约 30.4 秒；普通短轮约 2 秒。可能是模型冷启动或上游波动，但 UI 没有首 token、排队或超时提示。建议记录首 token 时间、增加模型健康检查和超时文案，并把上游延迟与 prompter/session 开销分开统计。

### P2-2 Facts 面板默认过窄

`system_prompt`、`runtime_context`、`context_snapshot` 默认被截断，需要手动拉宽或逐条展开。建议更宽的默认右栏、详情抽屉、复制按钮；长文本支持换行而不是省略号。

### P2-3 会话标题重复

两个测试会话都叫“只回答：OK”，列表只能靠记录数区分。建议重复标题自动加序号、模型或时间后缀；保留现有重命名入口。

### P2-4 删除会话的确认与重连需人工复现

自动化点击删除后页面进入“重连中…”，会话实际仍存在。可能是原生确认框阻塞了自动化，也可能是删除后事件流不稳定。建议改为应用内确认模态；删除成功后稳定选中相邻会话并刷新列表。

### P2-5 诊断面板交互与清理

插件中心打开时点击“诊断”没有立即出现面板，关闭插件中心后才出现；诊断计数没有清理入口。建议修正层级并提供 dismiss/clear。

### P2-6 长会话读取放大

每轮链路里 agent 调 `session.messages`，prompter 又调 `session.facts`，更新基线后还会再读一次；都是整文件读取。当前 26 facts 无问题，长会话会放大成本。建议增加增量 facts 接口或 prompter 内存缓存；compact 落地时一并处理。

### P2-7 首轮多一条 runtime_context

`session.create` 的 runtime context 使用 create payload 的 workspace（正斜杠），首轮 `session.info` 返回规范路径（反斜杠），因此每个新会话会多一条 runtime fact。只影响首次记录，不改变投影，但会增加 facts 噪声。可在 create 前统一路径格式，或首轮不写 runtime context。

## 3. 建议顺序

1. 确认产品边界：工具型 agent 还是纯对话。
2. 若工具型：先补 P1-6 工具插件 manifest，再做 P0-1 工具链。
3. 修 P0-2 刷新恢复。
4. 修 P1-1 取消上下文、P1-2 模型失败恢复、P1-3 错误展示。
5. 补 P1-4/P1-5 的消费链与 prompter 配置可见性。
6. 最后做 P2 的延迟、面板、标题和长会话性能。

## 4. 验证清单

- 新建会话、两轮追问：facts 顺序和 `surface_seqs` 正确；
- 文件读取请求：要么真实返回 README，要么明确说明无工具；
- 生成中刷新：后台完成后当前会话自动出现 assistant；
- 取消长文：下一轮不携带被取消的 user message；显式重试可用；
- 模型端点不可用：能换模型重试，不产生死会话；
- 错误：只出现一次，诊断能定位到 llm-openai；
- 插件中心：能看到 prompter 的 consumes 边与配置入口；
- 构建：`dist/plugins` 与 `shipped-plugins.conf` 一致；
- `go test ./...`、`go vet ./...` 通过。

## 5. 测试环境副作用

- 测试创建的 4 个会话已通过 `session.delete` 删除；既有会话未动。
- 测试服务已停止。
- 外部 Edge 的自动化页签在点击删除会话时被确认框卡住，可能需手动关闭；这是自动化操作的副作用，删除流程本身需按 P2-4 人工复现。
- 本轮构建把没有源码 `plugin.json` 的工具插件目录从 `dist` 裁剪掉；恢复前不要把这 4 个插件的行为当作当前实现。

## 6. 文件索引

| 位置 | 与本清单的关系 |
|---|---|
| `plugins/prompter/functions.go` | `session.create` / `llm.stream` 消费，透传与失败处理 |
| `plugins/prompter/context.go` | facts 读取、投影、snapshot 写入 |
| `plugins/prompter/prompt.go` | 静态/动态轨渲染、skills/MCP 列表 |
| `plugins/prompter/config.go` | 配置校验与默认值 |
| `plugins/agent/functions.go` | Turn 编排、`session.messages`、`llm.stream` 调用 |
| `plugins/agent/types.go` | `llm.stream` 的 meta |
| `plugins/session/functions.go` | `session.create` 初始 facts、facts/messages 投影 |
| `plugins/session/ui/store.js` | 重连、会话选择、UI 刷新 |
| `plugins/session/ui/chat.js` | 流式、停止、错误展示、发送 |
| `plugins/session/ui/trace.js` | facts 面板 |
| `plugins/llm-openai/functions.go` | 上游错误、流式 chunk、未来 tools |
| `internal/webui/plugin_snapshot.go` | 插件中心图数据 |
| `scripts/build.ps1` / `scripts/build.sh` | 插件剪裁与 dist 生成 |
| `scripts/shipped-plugins.conf` | 发行插件清单 |
