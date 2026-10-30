状态：当前实现

# Session

Session 保存工作区、会话元数据和事实记录。

## 创建与初始 facts

`session.create` 可带 `facts`。
Session 在建日志后、注册会话前按序写入这批事实。
写入失败时删除刚创建的文件并返回错误。
Session 只要求 `type` 非空，不解释事实含义。

## 会话模型

`session.create` 可写入 `model`。
`session.info` 和会话列表返回该字段。
已有会话保持创建时的模型，不提供修改接口。

Session Chat 的模型下拉框只用于新会话。
发送 `loop.turn` 时把所选模型 ID 写入 `model`。

