# session

`session` 是 Session Fact Log 的唯一业务真源。它保存 append-only 历史，并提供查询和模型上下文投影。Session Runtime 还负责驱动 Session WebUI。

## 1. 能力

| Capability | 说明 |
|---|---|
| `session` | Session 生命周期、Fact 和投影 |

Session 不调用 LLM，也不执行工具。

## 2. 持久化

一个 Session 对应一个 JSONL 文件：

```text
plugins/session/sessions/<session_id>.jsonl
```

每行一个 Fact。`seq` 在 Session 内单调递增。损坏行读取时跳过并写警告。

## 3. Fact 真源

最终 assistant `message` Fact 是正文真源。LLM delta 不写入 Session。刷新后从 Fact 重建 UI。

## 4. 配置

当前配置：

```json
{
  "fullToolResults": 8
}
```

该值控制 `session.derive` 保留完整结果的最近工具消息数量。更早的工具结果替换为摘要。

## 5. WebUI

Session 拥有：

| Component | Slot | 职责 |
|---|---|---|
| `session-history` | left | 会话列表和元数据 |
| `session-chat` | center | 消息、Turn 和 Stream Draft |
| `session-trace` | right | Fact Trace |

`session.state` 负责重建三个 UI State。`session.stream.update` 更新临时 Stream Draft。最终 assistant `message` Fact 到达后，Session 清除 Draft。

Session Runtime 是这三个 Component 的直接驱动者。llm-openai 不直接渲染输出。

当前交互包括会话搜索、按会话保留输入草稿、底部消息输入框、流式状态、停止生成、消息和代码复制、思考过程折叠、回到最新和稳定 revision。

## 6. 文档

详细契约见 [communication.md](communication.md)。该文档也记录前端通信。
