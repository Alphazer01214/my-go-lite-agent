# model-context

`model-context` 把每次请求的上下文贡献写进 Session，再从 Session 投影出实际模型输入。它遵循「模型可见 ⟺ 已记录」。

## 职责

- 消费 `llm.complete`，改写 `messages` / `tools` 内容区。
- 把渲染后的 system prompt 写成 Session Fact，内容变化才追加。
- 把 workspace、platform、mode、scheme、AGENTS.md 写成追加式 runtime context 快照。`platform` 取自宿主进程的 `runtime.GOOS`。
- 写入紧凑 `context_snapshot`，记录实际 surface、工具、hash、截断工具 seq 和本次模型窗口。
- 超预算时写确定性摘要与压缩 Fact，再重新投影。有效预算取 `min(max_context_tokens, context_window)`。
- 提供 `build` 干跑、`inspect` 按快照/session 还原消息和 `compact` 手工压缩。

## 配置

`config.json` 缺省时使用内置值：

```json
{
  "prompt_sections": [
    { "name": "identity", "order": -1000, "text": "You are an AI agent powered by LiteAgent." },
    { "name": "persona", "order": 0, "text": "You are a coding agent. Your working directory is {{workspace}}." },
    { "name": "policy", "order": 500, "text": "Use tools when they are needed..." }
  ],
  "max_context_tokens": 60000,
  "compact_threshold": 0.8,
  "retain_recent_tokens": 12000,
  "compact_max_chars": 12000,
  "agents_md_max_bytes": 65536,
  "snapshot_memory_limit": 128
}
```

变量只支持 `{{workspace}}`、`{{workspace_id}}`、`{{mode}}`、`{{scheme}}`。未知变量返回 `bad_arguments`，不调用模型。

`max_context_tokens` 是配置上限。请求的 `meta.context_window` 大于 0 时，实际预算取两者较小值；
窗口未知时只使用配置值。

## 方法

| 方法 | 用途 |
|---|---|
| `llm.complete`（消费） | 组装上下文并继续流水线 |
| `model-context.build` | 干跑下一次请求，不写 Fact |
| `model-context.inspect` | 按内存快照或 session 最新快照还原消息 |
| `model-context.compact` | 手工写入确定性摘要 |

## 依赖

- `session`：读写 Facts、调用 `session.derive`。
- `tools`：仅 `build` 干跑时在缺少工具输入时调用 `tools.list`。
- 两者都是弱依赖。llm.complete 路径要求 `session` 可用，否则不调用模型。

## 当前限制

- 不做 skills、memory、MCP。
- 不做 LLM 生成摘要，只做确定性摘要。
- `inspect` 的 `model_context_id` 分支只覆盖当前进程；`session_id` 分支读取持久 Facts，但消息正文不复制进快照。
- 旧快照缺少 `truncated_seqs` 时，inspect 用当前 derive 推断截断 seq；只有 hash 对上才采用。
- 不做 mode 专属 system prompt。

完整契约见 [communication.md](communication.md)。
