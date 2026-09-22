# llm-openai

OpenAI 兼容 LLM Provider（DeepSeek 等），流式输出。

## 提供

- Capability `llm`：`complete` / `info` / `stats`
  - `stats {sessionId}` → 模型名、请求数、模型总时长 / 平均 / 最后一次、token 数（进程内观测，底栏 chip 用）
- Config：`apiKey` / `baseURL` / `model` / `contextWindow`
- 命令：`/llm-openai config [get|set key=value]`

## 配置

`config.json`（与可执行文件同目录；密钥勿提交 git）。环境变量优先：

- `OPENAI_API_KEY` / `OPENAI_BASE_URL` / `OPENAI_MODEL` / `OPENAI_CONTEXT_WINDOW`

## UI

- `llm-openai-settings` —— Settings 浮层里本插件的设置面
- `llm-openai-status` —— Host 底栏信息区 chip（model / model-time / calls / tokens）

## Manifest

- `autostart: true`（核四件之一）
