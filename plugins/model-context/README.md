# model-context

`model-context` 构建每次 Step 实际发送给模型的输入。

## 职责

- 读取 `session.derive` 投影。
- 读取 `tools.list` 并转换成模型 Tool Definition。
- 合并 System Prompt 和 Workspace `AGENTS.md`。
- 应用 Allowed、Denied 和字符预算。
- 写入 `context_snapshot` Fact。
- 提供确定性摘要压缩。

## 配置

`config.json`：

```json
{
  "system_prompt": "You are LiteAgent...",
  "max_context_chars": 240000
}
```

## 方法

| 方法 | 用途 |
|---|---|
| `model-context.build` | 构建并持久化 Snapshot |
| `model-context.inspect` | 读取当前进程内 Snapshot |
| `model-context.compact` | 写入激活的 `context_summary` |

`inspect` 只覆盖当前进程生命周期。完整快照始终保存在 Session Fact 中。

完整契约见 [communication.md](communication.md)。
