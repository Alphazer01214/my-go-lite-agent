状态：当前实现

# Agent

Agent 接收 WebUI 输入，并驱动 Session、Prompter、LLM 与 Tools。

## 方法

| 方法 | 说明 |
|---|---|
| `loop.turn` | 执行一轮用户请求的 Agent Loop |
| `loop.cancel` | 取消当前会话的执行 |
| `agent.settings.get` | 读取 Agent 设置 |
| `agent.settings.save` | 保存 Agent 设置 |

`loop.turn` 接收 `workspace`、`session_id`、`user_input` 和 `model`。
已有会话使用创建时的模型。

## Agent Loop

Agent 按 `max_steps` 执行模型与工具循环。

1. 调用 `llm.stream`，只传模型选择和会话元数据。
2. 将助手消息写入 Session。
3. 响应带 `usage` 时追加一条 `type=usage` 的 Fact。
4. 没有 `tool_calls` 时返回最终回答。
5. 有 `tool_calls` 时按批次调用 `tool.call`。
6. 将工具结果写成 `role=tool` 的 message Fact。
7. 工具结果落库后继续下一轮模型调用。

工具调用达到 `max_steps` 仍未结束时返回 `max_steps_exceeded`。

## 工具并行

`tool_parallelism=auto` 时，同一模型步内的连续安全调用并发执行；其他调用形成屏障，单独执行。
工具结果始终按 `tool_calls` 原顺序写入 Session，与完成顺序无关。
并发上限由 `max_parallel_tools` 控制。

默认安全集是 `file_read`、`file_glob`、`file_grep` 和 `subagent`。
`shell_execute`、`python_execute`、`mcp_call`、`file_write` 和 `file_edit` 默认串行。
`subagent` 并行时会共享同一个 workspace；只应并行相互独立、不会同时修改同一批文件的任务。

`serial` 模式完全保持旧的逐调用行为。

## Usage

`usage` Fact 的 `role` 为 `host`。
`meta` 包含 `turn_id`、`step`、`model`、`prompt_tokens`、`completion_tokens` 和 `total_tokens`。
每次模型调用记录一条；供应商未返回 `usage` 时不写 Fact。
Agent 不估算 token。

## 下游请求

Agent 不组装模型可见内容。
`llm.stream` 的 `meta` 包含 `session_id`、`turn_id`、`workspace` 和 `model`。
Prompter 根据 Session Facts 装配 `messages` 和 `tools`。

助手消息的 `text`、`reasoning` 和 `tool_calls` 都写回 Session。
工具结果保留 `tool_call_id`、`is_error`、`truncated` 和 `meta`。

## Settings

| 字段 | 说明 |
|---|---|
| `max_steps` | 一次 `loop.turn` 的模型调用次数上限，正整数 |
| `tool_parallelism` | `serial` 或 `auto`，默认 `auto` |
| `max_parallel_tools` | 同时执行的工具数上限，默认 4，范围 1～8 |
| `parallel_safe_tools` | 可并行的工具名列表，默认 `file_read`、`file_glob`、`file_grep`、`subagent` |

设置持久化到插件目录的 `config.json`。
`parallel_safe_tools` 目前只支持配置文件修改，不在设置表单中展示。
保存设置时会保留未提交的字段。

