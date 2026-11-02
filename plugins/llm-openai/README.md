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

`llm.stream` 的 `model` 是模型 ID。
省略时使用 `default_model`。
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

## 模型能力

每个模型只声明图像输入能力：

| 字段 | 说明 |
|---|---|
| `vision` | 图像输入 |

`llm.models.list` 返回该能力，但不返回 API Key。

## Settings

Settings 管理 `models` 和 `default_model`。
模型 ID 必须唯一，`base_url` 必须非空，`default_model` 必须存在。
API Key 不回显；保存时留空表示保持原值。
