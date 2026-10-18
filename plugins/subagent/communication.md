# subagent 通信契约

## 1. run

Provider 输入沿用 Tool Provider 包络。`arguments`：

```json
{
  "task": "独立审查当前分支",
  "workspace": "E:/project",
  "parent_session_id": "s-parent",
  "allowed_tools": ["file_read", "file_search"],
  "max_steps": 16
}
```

输出：

```json
{
  "task_id": "sa-1",
  "session_id": "s-child",
  "reply": "审查结果",
  "steps": 4,
  "cancelled": false,
  "context_id": "ctx-1",
  "parent_session_id": "s-parent"
}
```

## 2. cancel

```json
{"task_id": "sa-1"}
```

```json
{"task_id": "sa-1", "cancelled": true}
```

取消会中断对 `agent.loop.turn` 的等待，并沿 Go Context 传播。

## 3. status

```json
{"task_id": "sa-1"}
```

输出字段：`task_id`、`parent_session_id`、`child_session_id`、`status`、`started_at`、`finished_at`、`error` 和 `delegation_depth`。

`status` 取值：`running`、`completed`、`failed`、`cancelled` 或 `not_found`。

## 4. 错误

| 错误码 | 场景 |
|---|---|
| `subagent_parent_invalid` | 父 Session 不存在 |
| `subagent_depth_exceeded` | 超过最大委派深度 |
| `call_cancelled` | 队列等待或运行被取消 |
