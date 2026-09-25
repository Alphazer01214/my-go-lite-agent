# llm-openai

无状态 OpenAI 兼容补全。不读写 session，不 Call 其它插件。

## 提供的能力

| capability.method | 说明 |
|-------------------|------|
| `llm.complete` | 补全；默认流式 |
| `llm.config.get` | 查看配置（密钥掩码） |
| `llm.config.set` | 修改配置（working 时挂起） |

## 流式

归因 evt `llm.chunk`：

- `{"channel":"content","delta":"…"}`
- `{"channel":"reasoning","delta":"…"}`
- `{"channel":"tool_call","index":0,"id":"…","function":{…}}`（OpenAI delta.tool_calls 形状）

## 配置

`config.json`：`base_url` / `api_key` / `model`。  
启动时 `OPENAI_BASE_URL` / `OPENAI_API_KEY` / `OPENAI_MODEL` 优先。  
`config.set` 在 complete 在途时排队，空闲后生效并写盘。
