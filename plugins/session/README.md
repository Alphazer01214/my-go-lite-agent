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

## 会话选项卡

Session Chat 顶部显示已打开会话的选项卡。
新建会话标签在第一次 `loop.turn` 返回 `session_id` 后替换为真实会话。
标签状态按工作区保存在 `sessionStorage`。
刷新后恢复标签和当前标签。
关闭标签不删除会话；运行中的标签不可关闭。

## 回合终态

Session Chat 根据 `turn_end` 事实渲染失败和取消终态。
失败回合显示错误详情和重试按钮。
重试会重发该回合的用户输入，不清空输入框草稿。
取消回合显示“本轮已取消”，并保留已经写入的助手消息。


## 消息 reasoning

message fact 可保存 `reasoning`。
`session.messages` 把它随助手消息返回。
Session Chat 在正文中折叠渲染思考过程。

## 工具消息

message fact 可保存 `tool_calls` 和 `tool_call_id`。
Agent 用 `role=tool` 的 message Fact 保存工具结果。
工具结果的 `is_error`、`truncated` 和 `meta` 保留在 Fact 中。

## Facts 复制与导出

Session Trace 支持逐条多选 facts。
全选只作用于当前筛选结果。
复制和导出只包含已选 facts。
两者使用相同的 JSON 内容。
JSON 包含 `session_id`、`exported_at`、`fact_count` 和 `facts`。
