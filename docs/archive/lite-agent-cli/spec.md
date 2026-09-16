# Lite Agent CLI（可用 CLI 工具）

Status: ready-for-agent

## Problem Statement

运行时骨架（Host + Session + Loop + Tools + Context Manager + Subagent）已齐，但只有 `fakellm` 与一次性 `-turn`：无法真实对话、无模型配置、无多轮交互、无过程态呈现。需要打通「CLI 工具级」可用路径：真实 OpenAI 兼容 LLM、Host 内建 REPL 状态机、流式输出、默认 assembly 与文档。

## Solution

1. **`llm-openai` 插件**：OpenAI 兼容 Chat Completions（可配 baseURL/apiKey/model），env 优先、插件目录 `config.json` 兜底；流式 delta 用 `EmitTo` 发 `presentation.stream`。
2. **三平面呈现**（对齐 dsh）：durable Session Log；ephemeral `presentation.stream`（不入日志）；pure `presentation.card`。Host 在 turn/step 边界发 `presentation.status`。
3. **`CallStream` 边收边回调**：CLI 实时打印 chunk；settlement 以 derive 的 assistant 文本为准。
4. **`host.exe -repl`**：stdin 循环 `RunTurn`，同进程同 Session；Ctrl+C 干净关插件。
5. **examples**：`chat.json`（session+llm-openai+contextmanager）、`agent.json`（+filetools）；README 改真 LLM 路径。

术语以 `CONTEXT.md` 为准（新增 Render Medium）；约束以 ADR-0001–0006 为准。不做 Session 持久化。

## User Stories

1. 作为使用者，我想设置 `OPENAI_API_KEY`（及可选 BASE_URL/MODEL）后启动 host，用真实模型对话。
2. 作为使用者，我想在 `-repl` 里连续输入多轮，Agent 记得同一 Session 内的上文。
3. 作为使用者，我想在模型生成时看到流式文字，而不是 turn 结束后一次性出现。
4. 作为使用者，我想工具执行时看到状态提示，结束后看到 Presentation Card。
5. 作为部署者，我想用插件目录 `config.json` 配置 baseURL/model（无 env 时）。
6. 作为使用者，我想 `examples/chat.json` 纯对话、`examples/agent.json` 带 filetools 开箱即用。
7. 作为插件作者，我想经 `presentation.stream` / `status` / `card` 向 Render Medium 发信号，而不关心 CLI 内部状态机。

## Implementation Decisions

- **Render Medium = Host 内建 CLI**（本票）；信号形状按多消费者订阅设计，不把 UI 焊死在 Host 私有结构。
- **三类 presentation method**：`stream`（ephemeral start/chunk/end）、`status`（idle/running）、`card`（现状纯投影）。stream/status **不入** Session Log。
- **Settlement**：chunk 实时打印；turn 结束后 assistant 正文以 `derive` / TurnResult 为准（与 dsh committed 语义一致）。
- **`llm-openai`**：stdlib `net/http`；配置 env（`OPENAI_API_KEY`/`OPENAI_BASE_URL`/`OPENAI_MODEL`）覆盖 `config.json`；`request_header` 上报真实 provider/model。
- **Assembly per-plugin 参数仍不做**；配置只走 env+文件。
- **持久化 / Inbox / 外置 UI 插件**：Out of Scope。

## Testing Decisions

- 主缝不变：真实 Host + fixture 插件进程。
- `llm-openai`：可用 `httptest` 假 OpenAI 服务做集成测（或 fixture 回放）；无 key 时 fail-loud 可诊断。
- REPL：主缝测「两轮输入同 Session，第二轮 Model Context 含第一轮」；流式至少断言 chunk 在 res 前到达（或 onChunk 被调）。
- Card/status：现有 presentation 测试不回归；REPL 路径可打印 status。

## Out of Scope

- Session JSONL 持久化 / resume。
- 独立 Web/Desktop Render Medium。
- Assembly per-plugin config。
- 真实多 provider 目录、token 计量、compaction。
- Windows TUI 富渲染（ANSI 进度条可后置）。

## Further Notes

- 实现顺序：01 llm-openai → 02 stream 回调+presentation 接线 → 03 -repl → 04 examples/contextmanager 默认段 → 05 README。
- `fakellm` 与测试 fixture 保留，不删除。
- 后续 tickets：`.scratch/lite-agent-cli/issues/`。
