状态：当前实现

# LLM OpenAI

LLM OpenAI 调用 OpenAI-compatible Chat Completions 接口。

## 方法

| 方法 | 说明 |
|---|---|
| `llm.stream` | 流式生成一条助手消息 |
| `llm.models.list` | 返回可用模型目录 |
| `llm.settings.get` | 读取模型设置 |
| `llm.settings.save` | 保存模型设置 |

`llm.stream` 接收 `model`、`messages` 和 `tools`。
`model` 省略时使用 `default_model`。
OpenAI 请求在顶层携带 `model`，不写入 `messages`。

## 流式事件

`llm.stream` 发出 `llm.chunk` 事件。
事件的 `channel` 只有 `thinking` 和 `content`。

| Channel | 说明 |
|---|---|
| `thinking` | 模型 reasoning 增量 |
| `content` | 助手正文增量 |

`thinking` 读取 `delta.reasoning_content`。
兼容接口也可返回 `delta.reasoning`。
最终助手消息的 `reasoning` 保存完整思考内容。
reasoning 只用于展示和留存，不写入后续请求正文。

## Tool Calls

Tool 定义转换为 OpenAI `tools` 字段。
流式 `delta.tool_calls` 会跨 chunk 累积。

最终助手消息保存结构化 `tool_calls`。
`function.arguments` 在内部保存为 JSON 对象。
历史助手消息重新请求时转换回 OpenAI 字符串参数格式。

`role=tool` 消息使用 `tool_call_id` 对应原调用。

## 模型能力

每个模型只声明图像输入能力：

| 字段 | 说明 |
|---|---|
| `vision` | 图像输入 |

`llm.models.list` 返回该能力，但不返回 API Key。

## Settings

Settings 管理 `models`、`default_model` 和三个流式保护参数。
模型 ID 必须唯一，`base_url` 必须非空，`default_model` 必须存在。
API Key 不回显；保存时留空表示保持原值。

| 字段 | 说明 |
|---|---|
| `models[].max_tokens` | 可选；0 表示不下发 `max_tokens`，兼容不支持该字段的端点 |
| `request_timeout_ms` | 单次请求总时限，默认 900000（15 分钟） |
| `stream_idle_timeout_ms` | 连续无 chunk 的中断时限，默认 120000（2 分钟） |
| `max_response_bytes` | 单次响应最多读取的字节数，默认 4194304（4 MiB） |

## 错误码

| 错误码 | 场景 |
|---|---|
| `llm_upstream_error` | 上游连接失败、非 2xx 或流解析失败；传输细节只写 Host 日志 |
| `llm_timeout` | 请求总超时或流空闲超时 |
| `llm_output_limit_exceeded` | 响应超过 `max_response_bytes` |

错误信息由调用方通过 `turn_end.meta.error_code` 保留，UI 负责显示友好文案。
