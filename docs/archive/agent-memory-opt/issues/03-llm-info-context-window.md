# 03 — llm.info：Context Window

**What to build:** llm-openai 实现 `llm.info` → `{contextWindow, model, provider}`。config 字段 `contextWindow`；env `OPENAI_CONTEXT_WINDOW` 覆盖；默认 65536。Host Loop 在 prepare 前取 window 并传入 `context.prepare`。

**Status:** resolved

- [x] llm.info + 配置覆盖
- [x] Host 传递 contextWindow
- [x] 主缝测试 `TestLLMInfoContextWindow`

## Answer

`llm.info` + `OPENAI_CONTEXT_WINDOW` + config.json `contextWindow`；`runTurn` 启动时 `LLMInfo()` best-effort。

## Comments

（待填）
