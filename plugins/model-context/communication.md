# model-context 通信契约

本文是 `model-context` 的跨插件 JSON 规范。调用方按本文维护自己的适配类型。

`model-context` 消费 `llm.complete`，把上下文贡献写进 Session，再从 `session.derive` 取得实际模型输入。

## 1. Manifest

```json
{
  "provides": ["model-context"],
  "consumes": [{ "capability": "llm", "method": "complete" }],
  "priority": 2000,
  "requires": ["session", "tools"]
}
```

`priority` 高于 session 与 llm-openai，保证组装发生在链首。

## 2. llm.complete 载荷

### 输入

```json
{
  "call_id": "t-1-s1",
  "messages": [{ "role": "user", "content": "分析这个项目" }],
  "tools": [],
  "stream": true,
  "meta": {
    "session_id": "s-1",
    "turn_id": "t-1",
    "step_id": "t-1-s1",
    "mode": "coding",
    "scheme": "tool_calling",
    "workspace_id": "ws-1",
    "workspace": "E:/project",
    "model": "deepseek-chat",
    "context_window": 65536
  }
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `meta.session_id` | 是 | 缺失返回 `bad_arguments`，不调用模型 |
| `meta.workspace` | 否 | 绝对路径；缺失时跳过 AGENTS.md 并记 warning |
| `meta.mode` / `meta.scheme` | 否 | 写入 runtime context |
| `meta.model` / `meta.context_window` | 否 | 写入快照；窗口用于约束压缩预算 |
| `messages` / `tools` | 否 | 内容区基线，model-context 整体替换 |
| 其他字段 | 否 | 原样保留并向下游传递 |

有效上下文预算取 `min(max_context_tokens, context_window)`。`max_context_tokens <= 0` 表示不设上限，
此时只受模型窗口约束；`context_window` 为 `0` 时只受配置约束。

### 输出

`model-context` 把 `messages` 换成 `session.derive` 的投影，`tools` 保持基线工具集，然后调用 `Proceed`。Response 原样返回。

## 3. Session Fact

### 3.1 system prompt

```json
{
  "type": "message",
  "role": "system",
  "content": "You are an AI agent powered by LiteAgent...",
  "meta": {
    "plugin": "model-context",
    "source": "model-context",
    "kind": "system_prompt",
    "hash": "64 位小写 SHA-256",
    "sections": [
      { "name": "identity", "order": -1000, "hash": "..." },
      { "name": "persona", "order": 0, "hash": "..." }
    ]
  }
}
```

- 只有渲染结果 hash 变化时追加。
- 其他来源的 system message 会折叠为 `Session instructions` 后进入最终 prompt。
- Session 投影把最新 `kind=system_prompt` 放到消息头部。

### 3.2 runtime context

```json
{
  "type": "message",
  "role": "user",
  "content": "Current runtime context. This snapshot supersedes earlier runtime-context snapshots.\n\n## Environment\nWorkspace: E:/project (ws-1)\nPlatform: windows\n\n## Mode\ncoding\n\n## AGENTS.md\n...",
  "meta": {
    "plugin": "model-context",
    "source": "model-context",
    "kind": "runtime_context",
    "hash": "64 位小写 SHA-256",
    "sections": [
      { "name": "environment", "hash": "..." },
      { "name": "mode", "hash": "..." },
      { "name": "agents_md", "hash": "..." }
    ]
  }
}
```

- 只有渲染结果 hash 变化时追加。
- 上下文从有到无时写 cleared 快照：

```json
{
  "type": "message",
  "role": "user",
  "content": "Current runtime context: none. Earlier runtime-context snapshots no longer apply.",
  "meta": {
    "plugin": "model-context",
    "source": "model-context",
    "kind": "runtime_context",
    "hash": "...",
    "sections": [],
    "cleared": true
  }
}
```

- Session 投影只保留最新一条 runtime context，并放在摘要之后。

### 3.3 context_snapshot

```json
{
  "type": "context_snapshot",
  "role": "host",
  "content": "",
  "meta": {
    "model_context_id": "mc-1760000000000000000-7",
    "revision": 7,
    "session_id": "s-1",
    "turn_id": "t-1",
    "step_id": "t-1-s1",
    "mode": "coding",
    "model": "deepseek-chat",
    "context_window": 65536,
    "surface": [3, 20, 4, 21],
    "message_count": 4,
    "messages_hash": "...",
    "truncated_seqs": [21],
    "tools": [],
    "tools_hash": "...",
    "token_estimate": 1234,
    "parts": [
      { "source": "system_prompt", "seqs": [3] },
      { "source": "summary", "seqs": [20] },
      { "source": "runtime_context", "seqs": [4] },
      { "source": "session", "seqs": [21] }
    ],
    "warnings": []
  }
}
```

| 字段 | 说明 |
|---|---|
| `model` / `context_window` | 本次请求的模型元数据；未知时省略 |
| `surface` | 实际发送的消息 Seq，按模型顺序排列 |
| `messages_hash` | OpenAI 消息形状的规范 JSON 的 SHA-256 |
| `truncated_seqs` | 被替换为确定性 stub 的工具消息 Seq |
| `tools` | 实际发送的工具定义 |
| `tools_hash` | 工具定义的 SHA-256 |
| `token_estimate` | 4 字符/token 固定估计 |
| `parts` | 内容来源与顺序 |
| `warnings` | 降级和截断说明 |

快照在 `Proceed` 之前写入。写入失败返回 `context_record_failed`，不调用模型。

### 3.4 context_summary

```json
{
  "type": "context_summary",
  "role": "host",
  "content": "Earlier conversation summary:\n- user: ...",
  "meta": {
    "active": true,
    "covers_through_seq": 42,
    "shadowed_from_seq": 7,
    "shadowed_to_seq": 42,
    "previous_summary_seq": 0,
    "plugin": "model-context",
    "generated_by": "model-context.compact",
    "strategy": "deterministic_digest",
    "reason": "budget",
    "compaction_id": "cc-...",
    "messages_covered": 18
  }
}
```

- 只覆盖普通对话消息，不覆盖 system prompt 与 runtime context。
- 按 user 消息分段，不切开 tool_call / tool_result 对。
- 最新 active 摘要生效。

### 3.5 context_compaction

```json
{
  "type": "context_compaction",
  "role": "host",
  "content": "",
  "meta": {
    "plugin": "model-context",
    "reason": "budget",
    "strategy": "deterministic_digest",
    "compaction_id": "cc-...",
    "source_from_seq": 7,
    "source_to_seq": 42,
    "messages": 18,
    "token_before": 80000,
    "token_after": 50000,
    "previous_summary_seq": 0
  }
}
```

摘要与压缩 Fact 在同一个 `session.append` 批次写入。

## 4. 方法

### 4.1 model-context.build

干跑组装，不写 Fact，不调用模型。

输入：

```json
{
  "session_id": "s-1",
  "turn_id": "t-1",
  "step_id": "t-1-s1",
  "mode": "coding",
  "scheme": "tool_calling",
  "workspace": "E:/project",
  "workspace_id": "ws-1",
  "model": "deepseek-chat",
  "context_window": 65536,
  "tools": [],
  "max_context_tokens": 0
}
```

- `tools` 省略时调用 `tools.list`；失败返回空工具并附 `tools_unavailable`。
- `max_context_tokens` 大于 0 时覆盖配置上限，但仍不会超过 `context_window`。

输出：

```json
{
  "model_context_id": "mc-preview-...",
  "messages": [],
  "tools": [],
  "token_estimate": 1234,
  "compact_needed": false,
  "warnings": [],
  "system_prompt_hash": "...",
  "runtime_context_hash": "..."
}
```

### 4.2 model-context.inspect

输入二选一：

```json
{ "model_context_id": "mc-1760000000000000000-7" }
```

```json
{ "session_id": "s-1" }
```

`model_context_id` 读取当前进程内存；找不到时返回 `model_context_not_found`。
`session_id` 通过 `session.query` 读取最新 `context_snapshot` Fact，不要求快照仍在内存。

输出：

```json
{
  "found": true,
  "snapshot": {
    "model_context_id": "mc-...",
    "session_id": "s-1",
    "surface": [3, 20, 4, 21],
    "messages_hash": "...",
    "truncated_seqs": [21],
    "tools": [],
    "token_estimate": 1234,
    "warnings": []
  },
  "messages": [
    {
      "seq": 3,
      "role": "system",
      "content": "You are an AI agent...",
      "source": "system_prompt",
      "token_estimate": 1200
    },
    {
      "seq": 21,
      "role": "tool",
      "content": "[tool result omitted; 120 bytes; see seq=21]",
      "truncated": true,
      "source": "session",
      "token_estimate": 12
    }
  ],
  "breakdown": {
    "system_prompt_tokens": 1200,
    "runtime_context_tokens": 80,
    "summary_tokens": 0,
    "session_tokens": 400,
    "tools_tokens": 900
  },
  "integrity": "ok",
  "warnings": []
}
```

还原规则：

1. 按 `surface` 顺序，从 Session 取对应 `message` / `context_summary` Fact。
2. `context_summary` 作为 system 消息，来源为 `summary`。
3. `kind=system_prompt` / `kind=runtime_context` 分别标记来源；其余为 `session`。
4. `truncated_seqs` 中的工具消息按 session 的 stub 格式重建，并标记 `truncated=true`。
5. 用重建的 OpenAI 消息计算 `messages_hash`，与快照比较；不一致时 `integrity="mismatch"` 并附 warning。
6. `breakdown` 和逐条 `token_estimate` 都是本地估算；`snapshot.token_estimate` 仍是整份请求的固定估算。

旧快照没有 `truncated_seqs` 时，inspect 会用当前 `session.derive` 的 `truncated_seqs` 重试。
只有重试后的 `messages_hash` 与快照一致才采用，并附 `truncation_inferred_from_current_derive` warning。

没有快照时返回 `{ "found": false, "messages": [] }`，不报错。
`session.query` 失败返回 `context_derive_failed`；按 ID 查询时若日志读取失败，仍返回快照并附 `reconstruction_unavailable` warning。

### 4.3 model-context.compact

输入：

```json
{
  "session_id": "s-1",
  "keep_recent_tokens": 12000,
  "max_summary_chars": 12000
}
```

输出：

```json
{
  "session_id": "s-1",
  "compacted": true,
  "source_from_seq": 7,
  "source_to_seq": 42,
  "summary_seq": 43,
  "messages_covered": 18,
  "token_before": 80000,
  "token_after": 50000
}
```

没有可压缩的旧分段时返回 `compacted: false`。

## 5. Session 依赖

`model-context` 要求 `session.derive` 输出：

- `messages[].seq`；
- `messages[].meta`，至少包含 `source` 与 `kind`；
- `covered_through_seq`、`summary_seq`；
- `truncated_seqs`，用于还原被 stub 的工具消息。

投影规则见 [session communication](../session/communication.md)。

## 6. 错误

| 错误码 | 场景 |
|---|---|
| `bad_arguments` | 缺少 `session_id`、inspect 两个 id 都缺失、meta 缺失、配置变量未知 |
| `context_derive_failed` | `session.derive` / `session.query` 失败 |
| `context_record_failed` | 写 system / runtime / snapshot / 摘要失败 |
| `context_budget_exceeded` | 压缩后仍超过 `max_context_tokens` |
| `model_context_not_found` | inspect 找不到内存快照 |
| `session_not_found` | 透传 Session 错误 |

记录失败时不调用模型。工具不可用不阻断。

## 7. 本轮不实现

- skills、memory、MCP。
- LLM 生成摘要。
- mode 专属 system prompt。
- 把消息正文复制进快照；inspect 仍从 Session Facts 还原。
- 自动 `context_resource` Fact。

## 8. 结构体索引

| Go 类型 | 角色 |
|---|---|
| `config` | 配置 |
| `completeMeta` | llm.complete 的 meta |
| `derivedMessage` / `deriveOut` | Session 投影 |
| `systemFactMeta` / `runtimeFactMeta` | 上下文 Fact meta |
| `snapshot` / `snapshotPart` | 请求快照 |
| `summaryFactMeta` / `compactionFactMeta` | 压缩 Fact meta |
| `buildIn/Out`、`inspectIn/Out`、`compactIn/Out` | 方法 payload |
| `assembly` / `prepared` | 内部组装结果 |
