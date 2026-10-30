状态：当前实现

# Session

Session 保存工作区、会话元数据和事实记录。

## 会话模型

`session.create` 可写入 `model`。
`session.info` 和会话列表返回该字段。
已有会话保持创建时的模型，不提供修改接口。

Session Chat 的模型下拉框只用于新会话。
发送 `loop.turn` 时把所选模型 ID 写入 `model`。

