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
3. 没有 `tool_calls` 时返回最终回答。
4. 有 `tool_calls` 时逐个调用 `tool.call`。
5. 将工具结果写成 `role=tool` 的 message Fact。
6. 工具结果落库后继续下一轮模型调用。

工具调用达到 `max_steps` 仍未结束时返回 `max_steps_exceeded`。

## 下游请求

Agent 不组装模型可见内容。
`llm.stream` 的 `meta` 包含 `session_id`、`turn_id`、`workspace` 和 `model`。
Prompter 根据 Session Facts 装配 `messages` 和 `tools`。

助手消息的 `text`、`reasoning` 和 `tool_calls` 都写回 Session。
工具结果保留 `tool_call_id`、`is_error`、`truncated` 和 `meta`。

## Settings

当前只提供 `max_steps`，限制一次 `loop.turn` 的模型调用次数。
取值必须是正整数，持久化到插件目录的 `config.json`。

