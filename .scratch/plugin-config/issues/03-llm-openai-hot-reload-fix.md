# 03 — llm-openai 配置热更新修复

**What to build:** `complete` 不再使用启动时捕获的 cfg；每次调用 `loadConfig()`（或 set 后更新内存）。验证 `config set model=` 后下一跳使用新 model。

**Status:** resolved

## Answer

complete/info/config.* 均在调用时 `loadConfig()`；闭包不再捕获 startup cfg。测试 `TestLLMOpenAIConfigSetPersistsModel`。

## Comments

（待填）
