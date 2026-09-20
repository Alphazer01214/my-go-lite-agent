# 01 — llm-openai 插件

**What to build:** 进程外 LLM 插件 `llm-openai`：OpenAI 兼容 `POST {baseURL}/chat/completions`，支持流式 SSE delta；配置 env 优先、插件目录 `config.json` 兜底。

**Blocked by:** —

**Status:** resolved

- [x] provides `llm`，方法 `complete`（+ presentation.stream chunk evt）
- [x] 配置：`OPENAI_API_KEY` / `OPENAI_BASE_URL`（默认 DeepSeek）/ `OPENAI_MODEL`；`config.json` 同名字段兜底
- [x] 无 apiKey 时 fail-loud、错误可诊断
- [x] 流式 SSE 解析 delta；非 SSE 回退整包 JSON
- [x] 主缝 httptest：假 OpenAI 服务，Host 一轮 turn 得到 assistant 文本

## Answer

`plugins/llm-openai`：stdlib HTTP；默认 `https://api.deepseek.com/v1` + `deepseek-chat`；env 覆盖 `config.json`。SSE 流式 `EmitTo(presentation.stream)`；缺 key 返回 `missing_api_key`。主缝 `TestLLMOpenAIRealTurn` / `TestLLMOpenAIMissingKey`。`config.example.json` 随插件分发。

## Comments

- API key 不入库；示例配置 apiKey 为空。
- tool_calls 支持流式聚合；模型名由用户配置（如 deepseek-flash）。
