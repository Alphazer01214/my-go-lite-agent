# agent 通信契约

本文是该插件跨插件 JSON 的规范来源。调用方按本文维护自己的适配类型。

本文记录 `agent` 的方法、payload、跨插件调用和 WebUI 交互。JSON 字段是公共契约。Go 私有类型名不是公共契约。

## 1. 对外方法

| 方法 | 用途 | 状态 |
|---|---|---|
| `loop.turn` | 执行一次 Turn | 当前实现 |
| `loop.cancel` | 取消 Session 的活动 Turn | 当前实现 |
| `agent.config.get` | 读取配置 | 当前实现 |
| `agent.config.set` | 更新配置 | 当前实现 |
| `agent.settings.get` | Settings Renderer 读取 | 当前实现 |
| `agent.settings.save` | Settings Renderer 保存 | 当前实现 |

## 2. loop.turn

### 输入

```json
{
  "input": "分析这个项目",
  "session_id": "s-123",
  "workspace_id": "ws-123",
  "workspace": "E:/project",
  "max_steps": 32,
  "allowed_tools": ["file_read"],
  "denied_tools": ["shell_execute"],
  "parent_session_id": "s-parent",
  "origin": "subagent",
  "delegation_depth": 1
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `input` | 是 | 用户输入 |
| `session_id` | 否 | 为空时创建 Session |
| `workspace_id` | 否 | 创建 Session 时绑定的 Workspace |
| `workspace` | 否 | Workspace 路径一致性校验 |
| `model` | 否 | 覆盖 Session 元数据里的模型 |
| `context_window` | 否 | 模型上下文窗口，供 model-context 约束压缩预算 |
| `max_steps` | 否 | 覆盖配置值 |
| `allowed_tools` | 否 | 非空时只保留交集工具 |
| `denied_tools` | 否 | 从交集或全集中移除工具 |
| `parent_session_id` | 否 | 子 Agent 的父 Session |
| `origin` | 否 | Session 来源，例如 `subagent` |
| `delegation_depth` | 否 | 委派深度 |

### Response

```json
{
  "session_id": "s-123",
  "reply": "最终正文",
  "steps": 2,
  "cancelled": false,
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 20,
    "total_tokens": 120
  },
  "seqs": [1, 2, 3]
}
```

`seqs` 是本 Turn 写入的 Session seq 列表。取消时保存已累计 assistant 正文，并写入 `turn_end(cancelled)`。
同一批次还会写入一条 `role=user`、`meta.kind=turn_cancelled` 的提示 Fact。
提示要求模型不要继续被取消的任务，避免下一轮误续写。

`loop.turn` 写入 `turn_start` 后发生错误时，当前实现会写入 `turn_end(error)`。`meta.error` 保存原始错误，稳定码保存在 `meta.error_code`。Browser Call 仍返回原始错误。

模型返回零个工具调用时按上游 `finish_reason` 判定终态：
`length` 返回 `llm_output_truncated`，`content_filter` 返回 `llm_content_filtered`，
`tool_calls` 或 `function_call` 却没有任何调用时返回 `llm_tool_calls_missing`。
以上三种都写 `turn_end(error)`，并保留已产生的正文。
`stop` 仍按 `completed` 记录，`turn_end.meta` 带 `tool_calls` 计数。

## 3. loop.cancel

输入：

```json
{
  "session_id": "s-123"
}
```

输出：

```json
{
  "session_id": "s-123",
  "cancelled": true
}
```

每个 Session 同时最多一个活动 Turn。`loop.cancel` 只取消该 Session 的活动 Turn。

## 4. Agent 配置

`agent.config.get` 输出：

```json
{
  "default_scheme": "tool_calling",
  "max_steps": 128
}
```

`agent.config.set` 输入字段均可省略：

```json
{
  "default_scheme": "tool_calling",
  "max_steps": 64
}
```

当前实现中，空字符串和非正数不覆盖原值。每次 Turn 在开始时捕获配置快照。配置修改不影响运行中的 Turn，并从下一次 Turn 开始生效。
新值同时写入插件目录下的 `config.json`；写盘失败时返回 `handler_error`，此时内存里的新值已经生效但没有持久化。

## 5. 下游调用

```mermaid
flowchart LR
    A[agent] -->|session.create| S[session]
    A -->|session.append| S
    A -->|session.derive| S
    A -->|session.stream.update| S
    A -->|llm.complete| M[model-context]
    M -->|Proceed| L[llm-openai]
    A -.->|tools.list/call| T[tools]
```

`model-context` 是 `llm.complete` 链上的可选消费者，agent 不再显式调用 `model-context.build`。
agent 只发送基线：`session.derive` 加 `tools.list`。model-context 活跃时会写 system / runtime Facts，再从 Session 重新投影并替换内容区。

`tools` 是弱依赖：能力缺失时跳过工具阶段。
回退不是静默的：`request_header` 的 `meta` 会带上 `degraded`，内容是降级原因（例如 `tools unavailable: …`）。

`llm.complete` 的 `meta`：

```json
{
  "session_id": "s-1",
  "turn_id": "t-1",
  "step_id": "t-1-s1",
  "mode": "tool_calling",
  "scheme": "tool_calling",
  "workspace_id": "ws-1",
  "workspace": "E:/project",
  "model": "deepseek-chat",
  "context_window": 65536
}
```

`model` 与 `context_window` 来自 Session 元数据，并在 Turn 开始时通过 `llm.models.list` 解析。
会话记录的模型仍在目录中时原样使用；已从目录移除时回退到当前默认模型，并在 `request_header.model_fallback_from`
记录原模型名。目录不可用时保留 Session 值。`context_window` 供 model-context 约束压缩预算。
本阶段 `mode` 与 `scheme` 同值。后续 agent 模式切片会改用独立的 mode。

Agent 每次 Turn 都调用 `session.info` 读取 Session 已绑定的 `workspace_id`、规范路径和模型元数据。

`workspace` 为空时直接使用 Session 值。显式给出时必须与 Session 值指向同一目录，否则返回 `bad_arguments`。Session 没有 Workspace 时同样返回 `bad_arguments`（消息为 `session workspace is required`）。

Agent 在 `tools.call` 中传递七个字段：`name`、`arguments`、`session_id`、规范 `workspace`、`workspace_id`、`mode` 和模型提供的 `call_id`。
Provider 应以 `session_id` 重新解析 Workspace，不信任调用路径。

## 6. Session Fact 序列

当前实现写出：

```text
turn_start
message(user)
step_start
request_header
[model-context: message(system), message(runtime_context), context_snapshot]
llm_usage
message(assistant)
message(tool)*
step_end
turn_end
```

方括号内的 Fact 由 model-context 在 `llm.complete` 中写入，只有内容变化时才追加。
`context_snapshot` 是模型实际输入的权威记录；`request_header` 是 agent 的基线摘要。

`turn_id`、`step_id` 和 `plugin` 写入对应 Fact 的 `meta`。

`request_header` 的 `meta` 字段：

| 字段 | 含义 |
|---|---|
| `step` | 当前步序号 |
| `mode` | 当前模式，本阶段等于 scheme |
| `scheme` | 当前工具方案 |
| `model` | 会话选择的模型；未选择时省略 |
| `context_window` | 会话选择的模型窗口；未选择时省略 |
| `token_estimate` | agent 基线消息与工具的估算；实际值以 `context_snapshot` 为准 |
| `degraded` | 仅在工具不可用时出现 |

`llm_usage` 的 `meta` 包含 `usage`、`model` 和 `context_window`。后两者来自 llm 的最终响应，表示实际执行的模型与窗口。
用量口径是上游 API 返回的 `prompt_tokens/completion_tokens/total_tokens`，不是本地估算。

`step_end` 的 `meta.reason` 取值：

| 值 | 含义 |
|---|---|
| `completed` | 本步没有工具调用，Turn 正常收束 |
| `tool_calls` | 本步有工具调用且全部执行完毕，继续下一步 |
| `cancelled` | 工具执行途中被取消，本步在此收口并结束 Turn |

工具循环中途取消时，assistant Fact 已经记下了本步的全部 `tool_calls`，但只有执行到的那几个有了结果。
未执行的调用会补写 `role=tool` 的取消结果（`is_error=true`、`cancelled=true`），保证每个
`tool_call_id` 都有对应结果；随后写 `step_end(cancelled)` 并结束 Turn，不再写 `step_end(tool_calls)`——
否则 Session 里会留下没有对应结果的 `tool_call_id`，下游按 id 配对时会错。

当前实现补充：

- `turn_id`、`step_id` 放入 Fact `meta`；
- `llm_usage` 写入 Session；
- assistant reasoning 写入最终 message 的 `meta`；
- 不写 `stream_chunk`；
- 取消时保存已累计 assistant 正文。

## 7. 流式通信

Agent 消费 `llm.chunk`，不再向浏览器透传该私有 payload。

当前链路：

```text
llm.chunk
→ Agent Stream Accumulator
→ StreamSnapshot
→ session.stream.update
→ session Runtime
→ Command(stream)
→ session-chat
```

`StreamSnapshot`：

```json
{
  "session_id": "s-123",
  "turn_id": "t-1",
  "step_id": "st-2",
  "update_id": 17,
  "message": {
    "role": "assistant",
    "content": "当前累计正文",
    "reasoning": "",
    "tool_calls": []
  },
  "status": "streaming"
}
```

`stream_chunk` 不持久化。Agent 会合并快速 Delta，首个 Delta 立即发布，之后最多每 24 个 Delta 或 100 ms 发布一次。最终 complete 和取消快照始终发布。最终 assistant `message` Fact 是正文真源。

## 8. WebUI 交互

| Component | 操作 | Capability |
|---|---|---|
| `session-chat` | 发送用户输入 | `loop.turn` |
| `session-chat` | 取消当前 Turn | `loop.cancel` |
| `session-chat` | 显示临时正文 | session 的 `Command(stream)` |
| `session-chat` | 显示最终正文 | session 的 `State(ChatState)` |

Agent 没有顶层 Component。Agent 不直接调用 `setData` 或 `runCommand`。

## 9. 错误

| 场景 | 行为 |
|---|---|
| `input` 为空 | `bad_arguments` |
| Session 创建或写入失败 | 返回错误 |
| LLM 调用失败 | 返回 `llm_upstream` 或下游错误 |
| 工具失败 | 写 `tool error [code]:` 和 `is_error`，`meta.error_code` 保存稳定码，继续 Turn |
| 上游截断 | `llm_output_truncated`，写 `turn_end(error)` |
| 上游内容过滤 | `llm_content_filtered`，写 `turn_end(error)` |
| 声称工具调用但结果为空 | `llm_tool_calls_missing`，写 `turn_end(error)` |
| 达到 `max_steps` | 最后一步仍有工具调用时写 `turn_end(max_steps)`；最后一步正常收束时写 `turn_end(completed)` |
| 取消 | 中止 LLM Call；工具执行途中取消时先写 `step_end(cancelled)`，再写 `turn_end(cancelled)` |
| `config.set` / `settings.save` 写盘失败 | 返回 `handler_error`，内存值已生效但未持久化 |
| 插件退出 | Host 合成 `plugin_down`；Session 可补写 `turn_end(error)` |

## 10. 结构体索引

| Go 类型 | 角色 |
|---|---|
| `turnState` | 单 Session 活动 Turn |
| `turnIn` | `loop.turn` 输入 |
| `turnOut` | `loop.turn` 输出 |
| `cancelIn/cancelOut` | 取消输入输出 |
| `config/configSetIn` | 配置 |
| `chatMessage` | LLM 上下文消息 |
| `toolCall/toolSpec` | LLM 和工具描述 |
| `llmCompleteIn/llmCompleteOut` | LLM 调用子集 |
| `sessionCreateIn/sessionAppendIn` | Session 调用子集 |
| `streamSnapshot/streamAccumulator` | 流式累积和 Session 更新 |
