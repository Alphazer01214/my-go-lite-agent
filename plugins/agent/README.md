# agent

Turn 编排者 + 插件组装唯一入口。一切消息经 agent；多 session 并发。

## 提供的能力

| capability.method | 说明 |
|-------------------|------|
| `loop.turn` | 一轮编排（跨 session 并行，同 session 串行） |
| `loop.cancel` | 取消本 session 在途 turn |
| `agent.config.get` / `set` | `default_scheme` · `max_steps`（working 挂起） |

## 编排

```text
session.append(user) → memory.assemble | session.derive
  → tools.list（scheme/allowed_tools 过滤）
  → llm.complete（chunk 经 agent Emit）
  → tool 循环 → session.append(assistant / turn_end)
```

可关：无 memory → session.derive；无 tools → 纯 chat。

## 配置

`config.json`：`default_scheme`（chat|tool_calling|coding）、`max_steps`（默认 128）。
