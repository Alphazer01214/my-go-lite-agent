# agent

`agent` 负责一次 Agent Turn 的编排。它组装上下文、调用 LLM、执行工具、管理取消，并把稳定结果写入 Session Fact Log。

## 1. 能力

| Capability | 说明 |
|---|---|
| `loop` | Turn 生命周期 |
| `agent` | Agent 配置 |

`plugin.json` 声明的硬依赖是 `session` 和 `llm-openai`。`tools` 是弱依赖能力：缺失时跳过工具阶段。`model-context` 是 `llm.complete` 链上的可选消费者，agent 不直接调用它。

## 2. 当前职责

- 创建或复用 Session；
- 以 Session 已绑定的 Workspace 作为本次 Turn 的唯一工作目录；
- 从 Session 读取本次使用的 `model/context_window`，并用 `llm.models.list` 对照当前目录：模型已从目录移除时回退到当前默认模型，回退来源写入 `request_header.model_fallback_from`；
- 写入 user message；
- 构建基线：`session.derive` 加 `tools.list`；
- 调用 `llm.complete`，由 model-context 在链上决定是否替换上下文；
- 执行 `tools.call` 并读取统一 Tool Result；
- 写入 assistant/tool message，以及工具错误和截断元数据；
- 写入 turn、step 和 usage 元数据；usage fact 同时记录实际模型与窗口；
- 处理取消和最大步数。

Agent 不保存 UI State，也不直接渲染消息。

## 3. 配置

```json
{
  "default_scheme": "tool_calling",
  "max_steps": 128
}
```

| 字段 | 默认值 | 说明 |
|---|---|---|
| `default_scheme` | `tool_calling` | 只有 `chat` 和 `tool_calling` 有区别：`chat` 不给模型挂工具，其余取值按 `tool_calling` 处理 |
| `max_steps` | `128` | 单 Turn 最大 Step 数 |

运行中的 Turn 使用开始时的配置快照。`config.set` 立即生效，并立即写入 `config.json`。
写盘失败时返回 `handler_error`：内存里的新值仍然生效，但没有持久化。

Settings Renderer 使用 `agent.settings.get` 和 `agent.settings.save`。保存仍进入同一配置快照。

minimal/normal 模式命名尚未落地；当前代码只区分 `chat` 与其余取值。

## 4. WebUI

第一阶段没有 agent 顶层 Component。

`session-chat` 通过 `loop.turn` 发起对话，通过 `loop.cancel` 取消。Agent 把流式增量发送给 `session.stream.update`，不直接驱动 `session-chat`、`session-history` 或 `session-trace`。

Agent 不接受调用方覆盖 Session Workspace。`loop.turn` 中显式传入的 `workspace` 只用于一致性校验，不一致时返回参数错误。

## 5. 文档

详细契约见 [communication.md](communication.md)。该文档也记录调用时序。
