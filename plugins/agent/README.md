状态：当前实现

# Agent

Agent 接收 WebUI 输入，并串联 Session 与 LLM。

## 方法

| 方法 | 说明 |
|---|---|
| `loop.turn` | 执行一次单轮对话 |
| `loop.cancel` | 取消当前会话的执行 |
| `agent.settings.get` | 读取 Agent 设置 |
| `agent.settings.save` | 保存 Agent 设置 |

`loop.turn` 接收 `workspace`、`session_id`、`user_input` 和 `model`。
已有会话使用创建时的模型。

## 下游请求

Agent 调 `llm.stream` 时带 `model`、`messages` 和 `meta`。
`messages` 是当前会话的消息投影，作为 Prompter 缺席时的兜底。
`meta` 包含 `session_id`、`turn_id`、`workspace` 和 `model`。

## Settings

当前只提供 `max_steps`。
取值必须是正整数，持久化到插件目录的 `config.json`。

