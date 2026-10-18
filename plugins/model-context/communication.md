# model-context 通信契约

## 1. build

输入：

```json
{
  "session_id": "s-123",
  "turn_id": "t-1",
  "step_id": "t-1-s1",
  "scheme": "tool_calling",
  "workspace": "E:/project",
  "allowed_tools": [],
  "denied_tools": [],
  "max_chars": 240000
}
```

`session_id` 必填。`allowed_tools` 非空时只保留交集。`denied_tools` 最后移除。

输出：

```json
{
  "model_context_id": "mc-1",
  "messages": [{"role": "user", "content": "hello"}],
  "tools": [],
  "token_estimate": 42,
  "warnings": []
}
```

构建成功后向 Session 追加 Fact：

```json
{
  "type": "context_snapshot",
  "role": "host",
  "content": "",
  "meta": {
    "model_context_id": "mc-1",
    "session_id": "s-123",
    "turn_id": "t-1",
    "step_id": "t-1-s1",
    "revision": 1,
    "messages": [],
    "tools": [],
    "parts": [],
    "token_estimate": 42,
    "hash": "64-hex"
  }
}
```

## 2. inspect

```json
{"model_context_id": "mc-1"}
```

返回完整 Snapshot。进程重启后返回 `model_context_not_found`。

## 3. compact

```json
{"session_id": "s-123", "keep_last": 20, "max_chars": 12000}
```

保留最后 `keep_last` 条 message，其余生成确定性摘要。写入一条激活的 `context_summary` 和一条 `context_compaction`。

输出：

```json
{
  "session_id": "s-123",
  "compacted": true,
  "source_from_seq": 10,
  "source_to_seq": 50,
  "summary_seq": 51,
  "messages_covered": 40
}
```

## 4. 降级

`tools` 不可用时仍构建消息，但不提供 Tool。`session` 不可用时构建失败。字符预算超出时从最旧的对话消息开始裁剪，System Prompt 保留。
