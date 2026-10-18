# agent

`agent` 负责一次 Agent Turn 的编排。它组装上下文、调用 LLM、执行工具、管理取消，并把稳定结果写入 Session Fact Log。

## 1. 能力

| Capability | 说明 |
|---|---|
| `loop` | Turn 生命周期 |
| `agent` | Agent 配置 |

`plugin.json` 声明的硬依赖是 `session`、`llm-openai`、`tools` 和 `model-context`。运行中仍保留降级路径：Model Context 不可用时读取 `session.derive`，Tools 不可用时跳过工具阶段。

## 2. 当前职责

- 创建或复用 Session；
- 写入 user message；
- 调用 `model-context.build` 获取模型输入和工具定义；
- 在 Model Context 不可用时回退到 Session 投影；
- 调用 `llm.complete`；
- 执行 `tools.call` 并读取统一 Tool Result；
- 写入 assistant/tool message，以及工具错误和截断元数据；
- 写入 turn、step 和 usage 元数据；
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
| `default_scheme` | `tool_calling` | `chat`、`tool_calling`、`coding` |
| `max_steps` | `128` | 单 Turn 最大 Step 数 |

运行中的 Turn 使用开始时的配置快照。`config.set` 立即保存，并从下一次 Turn 开始生效。

Settings Renderer 使用 `agent.settings.get` 和 `agent.settings.save`。保存仍进入同一配置快照。

minimal/normal 模式命名尚未落地。当前文档保持代码中的三种 scheme。

## 4. WebUI

第一阶段没有 agent 顶层 Component。

`session-chat` 通过 `loop.turn` 发起对话，通过 `loop.cancel` 取消。Agent 把流式增量发送给 `session.stream.update`，不直接驱动 `session-chat`、`session-history` 或 `session-trace`。

## 5. 文档

详细契约见 [communication.md](communication.md)。该文档也记录调用时序。
