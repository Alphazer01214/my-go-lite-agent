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

## 模型能力

每个模型可声明以下能力：

| 字段 | 说明 |
|---|---|
| `streaming` | 流式输出 |
| `tools` | 工具调用 |
| `vision` | 图像输入 |
| `audio` | 音频输入 |
| `reasoning` | 推理 |
| `json_schema` | JSON Schema 输出 |

`llm.models.list` 返回这些能力，但不返回 API Key。

## Settings

Settings 管理 `models` 和 `default_model`。
模型 ID 必须唯一，`base_url` 必须非空，`default_model` 必须存在。
API Key 不回显；保存时留空表示保持原值。

