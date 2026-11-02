状态：当前实现

# Prompter

Prompter 是模型请求装配器。
Session 只记录事实，Prompter 决定所有模型可见内容。

## 消费的方法

| 方法 | 行为 |
|---|---|
| `session.create` | 在 payload 中补 `system_prompt`、`runtime_context` 两类初始 fact，然后 `Proceed` |
| `llm.stream` | 读取会话 facts 和 Tool 定义，装配 `messages`、`tools` 后 `Proceed` |

两者都是透明消费。
`Proceed` 后最终响应和事件直达原调用方。
带 `session_id` 的模型请求必须经过 Prompter 装配。
装配失败时返回错误，不透传调用方正文。

## `llm.stream` 输入

Agent 在 payload 中只带 `meta` 和模型选择。
Prompter 在 `meta.session_id` 存在时装配全部模型可见内容。
无 `session_id` 的独立请求仍按原 payload 透传。

## Facts

| 类型 | Role | 内容 |
|---|---|---|
| `system_prompt` | `system` | 静态轨全文，`meta.hash` 记录内容哈希 |
| `runtime_context` | `user` | `<runtime_context>` 包裹的环境、skills、MCP |
| `context_snapshot` | `host` | 本次实际送出的 surface seqs、hash、token 估算；不投影 |

`message` 类型仍由 Agent 追加。
Prompter 只读 message fact，不修改已有消息。
message fact 的 `reasoning`、`tool_calls` 和 `tool_call_id` 随消息投影保留。
正文 hash 和 token 估算只计算 `text`。

## 装配顺序

1. 最新 `system_prompt`。
2. 最新 `runtime_context`。
3. 所有 `message` fact，按 seq 升序。
4. `tool.list` 返回的 Tool 定义，放入请求级 `tools`。

Tool 定义不写入 Session Facts。
`context_snapshot` 记录 catalog generation、hash 和工具名，便于定位问题。

## 配置

`config.json` 可选；缺省使用内嵌的 `config.example.json`。

字段：

- `prompt_sections`：`name`、`order`、`track`、`text`。
- `track` 取 `static` 或 `dynamic`；缺省按 `order` 推断，负数为 `static`。
- 静态轨禁止 `{{...}}`。
- 动态轨支持 `{{workspace}}`、`{{model}}`、`{{platform}}`、`{{date}}`。
- `skills`：`name`、`description`。
- `mcp_servers`：`name`、`description`、`tools`。

配置在进程启动时读取。
修改后重启插件生效。

## 失败行为

- 配置非法：插件启动失败，消费链回落到 Provider。
- 会话读取、渲染、Tool 获取或装配失败：返回错误。
- `context_snapshot` 写入失败只记录，不影响请求。

## 非职责

不持有会话历史；不调用模型；不执行工具；首版不做压缩。
