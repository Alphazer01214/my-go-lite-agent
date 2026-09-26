# agent 插件设计 — 里程碑

**状态**：方案已定稿；结构体待逐条确认。  
**范围**：`agent`（provides `loop`）。  
**交接**：需求完成后**删除本文**。

---

## 职责

**Agent = turn 编排者 + 插件组装唯一入口**。一切消息经 agent；不存对话真源、不调 HTTP、不执行工具本体。

```text
loop.turn
  → session.append(user)
  → memory.assemble 或 session.derive（memory 可关）
  → tools.list + scheme/allowed_tools 过滤（tools 可关）
  → llm.complete（收 chunk → agent.Emit(loop, chunk)）
  → tool 循环（append tool → 再 complete）
  → session.append(assistant / turn_end)
```

**多 session 并发**：跨 session 全并行；同 session 串行（per-session 锁）。

---

## 已定决策

| # | 点 | 结论 |
|---|----|------|
| 1 | capability | `loop.turn` · `loop.cancel` · `agent.config.get/set` |
| 2 | turn 入出参 | `input/session_id/workspace/max_steps=128/allowed_tools` → `session_id/reply/steps/cancelled/usage/seqs` |
| 3 | cancel + tool 循环 | cancel 仅本 session；无在途 `cancelled:false`；max_steps 到 → `turn_end{reason:max_steps}` |
| 4 | tools 归属 | **agent 交 llm**；memory 只做 system prompt + 记忆；**context-manager 改名 memory** |
| 5 | 流式 | **全经 agent**：收 llm.chunk → `Emit(loop, chunk)`；调用方只面对 loop |
| 6 | 并发锁 | per-session 互斥；config.set working 挂起 |

### 取消（v1 不改 Host）

`loop.cancel` 置 flag；步骤间短路；在途 `llm.complete` goroutine 弃结果；可选以后加 `llm.cancel`。

### 错误码（草案）

`bad_arguments` | `session_not_found` | `handler_error` | `max_steps`（turn_end reason） | `cancelled`

---

## 待确认（结构体）

1. `turnIn` / `turnOut` / `cancelIn` / `cancelOut`
2. `config`（default_scheme…）
3. 内部 `turnState` / registry
4. 核心函数
