# llm-openai

`llm-openai` 是无状态的 OpenAI-compatible LLM 适配器。它接收 messages 和 tools，返回最终结果，并在流式模式下发送 delta。

## 1. 能力

| Capability | 说明 |
|---|---|
| `llm` | Completion 和配置 |

`llm-openai` 不读取 Session，不编排 Turn，不渲染 WebUI 正文。

## 2. 配置

```json
{
  "base_url": "https://api.deepseek.com/v1",
  "api_key": "",
  "model": "deepseek-chat"
}
```

启动优先级：

```text
环境变量
→ config.json
→ 内置默认值
```

支持的环境变量：

| 环境变量 | 字段 |
|---|---|
| `OPENAI_BASE_URL` | `base_url` |
| `OPENAI_API_KEY` | `api_key` |
| `OPENAI_MODEL` | `model` |

真实 API Key 不进入 Git，也不进入 `config.json`：`llm.config.set` 落盘前会把 `api_key` 清空，
密钥只保留在进程内存和环境变量里，进程重启后需要重新提供。`config.json` 以 `0644` 写入，
所以它只应该放非敏感配置。`config.get` 返回部分脱敏 key。`llm.settings.get` 只返回是否已配置。

`base_url` 在文件、环境变量和 `config.set` 三条路径上都会归一化（去首尾空白和末尾 `/`）。

## 3. 前端交互

第一阶段没有 llm-openai 顶层 Component。

Host Settings 页面按统一 Schema 渲染：

| 字段 | 控件 |
|---|---|
| `base_url` | string |
| `api_key` | secret |
| `model` | string |

读取调用 `llm.settings.get`，保存调用 `llm.settings.save`。插件负责持久化。每次 `llm.complete` 使用调用开始时的配置快照。修改不影响运行中的 Call，并从下一次 Call 开始生效。

空 Secret 表示不修改 API Key。清除 Secret 需要插件后续提供显式操作。

## 4. 文档

详细契约见 [communication.md](communication.md)。该文档也记录上游协议。
