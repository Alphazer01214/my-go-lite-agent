# llm-openai 通信契约

本文是该插件跨插件 JSON 的规范来源。调用方按本文维护自己的适配类型。

本文记录 `llm-openai` 的方法、payload、Event 和前端通信。

## 1. 对外方法

| 方法 | 用途 | 状态 |
|---|---|---|
| `llm.complete` | OpenAI-compatible Completion | 当前实现 |
| `llm.cancel` | 取消指定 Call | 当前实现 |
| `llm.config.get` | 读取模型配置 | 当前实现 |
| `llm.config.set` | 更新模型配置 | 当前实现 |

## 2. llm.complete

### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "分析这个项目"
    }
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "file_read",
        "description": "Read a file",
        "parameters": {
          "type": "object",
          "properties": {
            "path": {
              "type": "string"
            }
          }
        }
      }
    }
  ],
  "model": "deepseek-chat",
  "temperature": 0.2,
  "max_tokens": 4096,
  "stream": true,
  "call_id": "t-1-s1"
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `messages` | 是 | OpenAI chat messages |
| `tools` | 否 | function tools |
| `model` | 否 | 覆盖配置 |
| `temperature` | 否 | 采样温度 |
| `max_tokens` | 否 | 最大生成 token |
| `stream` | 否 | nil 时默认 true |

### Response

```json
{
  "content": "最终正文",
  "reasoning": "最终推理",
  "tool_calls": [
    {
      "id": "call-1",
      "type": "function",
      "function": {
        "name": "file_read",
        "arguments": "{\"path\":\"README.md\"}"
      }
    }
  ],
  "finish_reason": "tool_calls",
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 20,
    "total_tokens": 120
  }
}
```

流式模式下，Response 仍返回累计后的完整结果。Event 只用于实时增量。

## 3. llm.cancel

输入：

```json
{ "call_id": "t-1-s1" }
```

输出：

```json
{ "call_id": "t-1-s1", "cancelled": true }
```

`call_id` 由 Agent 传入 `llm.complete`。取消会中止对应上游 HTTP 请求。

`cancelled` 表示"此刻确实有一个在途 completion 被中止了"。`call_id` 不在飞行中时返回 `cancelled: false`——
即使它随后才注册，取消仍会生效：`llm.cancel` 会把这个 id 记进一个有界缓冲（上限 256 条），
`llm.complete` 注册时发现它已被标记就立即中止自己。这样既覆盖了调用启动竞态，又不会因为大量无效取消而无限增长。

## 4. llm.chunk Event

Event 使用 Request 的 id。`capability.method` 是 `llm.chunk`。

### Text delta

```json
{
  "channel": "content",
  "delta": "你"
}
```

`channel` 可以是：

| channel | 内容 |
|---|---|
| `content` | 正文 delta |
| `reasoning` | 推理 delta |

### Tool call delta

```json
{
  "channel": "tool_call",
  "index": 0,
  "id": "call-1",
  "type": "function",
  "function": {
    "name": "read_file",
    "arguments": "{\"path\":"
  }
}
```

工具调用按 `index` 合并。`id`、`type` 和 `name` 可能在首个 delta 后补全。`arguments` 按顺序追加。

`llm.chunk` 是 llm-openai 私有传输格式。WebUI 不直接消费它。Agent 会把它累计成统一 Stream Snapshot。

## 5. 上游请求

非流式模式请求：

```text
POST {base_url}/chat/completions
Authorization: Bearer {api_key}
Content-Type: application/json
```

流式模式额外设置：

```text
Accept: text/event-stream
```

响应按 `data:` 行解析。`[DONE]` 结束流。单个非流式请求超时是 5 分钟。流式请求没有总超时。

## 6. 配置方法

### llm.config.get

当前输出：

```json
{
  "base_url": "https://api.deepseek.com/v1",
  "api_key": "api***key",
  "model": "deepseek-chat"
}
```

目标 Settings 契约改为：

```json
{
  "base_url": "https://api.deepseek.com/v1",
  "api_key_configured": true,
  "model": "deepseek-chat"
}
```

### llm.config.set

输入字段均可省略：

```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "new-secret",
  "model": "gpt-test"
}
```

当前实现中，空字符串不覆盖原值。每次 `llm.complete` 在调用开始时捕获配置快照。配置修改不影响运行中的 Call，并从下一次 Call 开始生效。Settings 空 secret 表示不修改。清除 secret 需要显式动作。

`base_url` 在三条输入路径（`config.json`、环境变量、`llm.config.set`）上都会去掉首尾空白和末尾 `/`，
因此不会拼出 `…/v1//chat/completions`。

新值会写入插件目录下的 `config.json`，但 `api_key` 落盘前被清空：密钥只留在内存和环境变量里。
写盘失败时返回 `handler_error`，此时内存里的新值已经生效但没有持久化。

## 7. 调用关系

```mermaid
sequenceDiagram
    participant A as agent
    participant L as llm-openai
    participant U as OpenAI-compatible API
    A->>L: llm.complete
    L->>U: POST /chat/completions
    U-->>L: SSE delta
    L-->>A: llm.chunk
    U-->>L: [DONE]
    L-->>A: completeOut
```

`llm-openai` 不调用 `session`。

## 8. 错误

| 场景 | 错误码 |
|---|---|
| `messages` 为空 | `bad_arguments` |
| JSON 解码失败 | `bad_arguments` |
| HTTP 建连或上游失败 | `llm_upstream` |
| 上游非 2xx | `llm_upstream` |
| JSON marshal 失败 | `handler_error` |
| `llm.config.set` 写 `config.json` 失败 | `handler_error` |

Event 发送失败不影响最终 Response。当前实现会忽略 emit error。

## 9. 前端通信

| UI 面 | 方向 | 方法 |
|---|---|---|
| Settings 读取 | UI → Plugin | `llm.config.get` |
| Settings 保存 | UI → Plugin | `llm.config.set` |
| Settings Renderer 读取 | UI → Plugin | `llm.settings.get` |
| Settings Renderer 保存 | UI → Plugin | `llm.settings.save` |
| LLM 正文 | Plugin → Agent | `llm.chunk` |
| LLM 最终结果 | Plugin → Agent | `llm.complete` Response |

llm-openai 没有 `setData` 或 `runCommand` 调用。

## 10. 结构体索引

| Go 类型 | 角色 |
|---|---|
| `chatMessage` | 模型消息 |
| `toolCall` | 工具调用 |
| `tool` | 工具描述 |
| `completeIn` | Completion 输入 |
| `completeOut` | 最终输出 |
| `chunkText` | content/reasoning delta |
| `chunkToolCall` | tool call delta |
| `config` | 磁盘和运行配置 |
| `configGetOut/configSetIn` | 配置方法 payload |
