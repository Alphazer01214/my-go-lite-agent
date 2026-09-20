# 04 — examples 与默认 System Prompt 段

**What to build:** `examples/chat.json`、`agent.json`；contextmanager 默认段；llm-openai 配置示例。

**Blocked by:** 01 — llm-openai

**Status:** resolved

- [x] chat.json: session + llm-openai + contextmanager
- [x] agent.json: chat + filetools
- [x] segments.json：identity / tools / safety
- [x] config.example.json + build.ps1 打包

## Answer

examples 与默认段已落盘；`scripts/build.ps1` 安装 llm-openai（timeoutMs=120000）、contextmanager、filetools 并复制示例。

## Comments

- 不实现 Assembly per-plugin 参数。
