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
`session.info` 和会话列表同时返回 `message_count`，它统计可投影为聊天气泡的 message Facts。
已有会话保持创建时的模型，不提供修改接口。

Session Chat 的模型下拉框只用于新会话。
发送 `loop.turn` 时把所选模型 ID 写入 `model`。

## 运行期元数据

Turn 运行期间，Session Chat 每 2 秒调用 `session.info` 和 `session.list`。
标题、消息数和 Facts 总数随落库进度更新，不重新加载完整消息和 Facts。
Turn 结束后仍由 `refreshCurrent()` 加载完整正文和 Facts 列表。

## 会话选项卡

Session Chat 顶部显示已打开会话的选项卡。
新建会话标签在第一次 `loop.turn` 返回 `session_id` 后替换为真实会话。
标签状态按工作区保存在 `sessionStorage`。
刷新后恢复标签和当前标签。
关闭标签不删除会话；运行中的标签不可关闭。

## Subagent 会话

Session History 默认显示 `origin=subagent` 的子会话，并用“子代理”标记区分。
子会话可以单独打开，查看自己的消息和 Facts。
父会话的 `subagent_spawned` / `subagent_finished` Facts 仍保留 `child_session_id`，可追踪派发关系。

## 回合终态

Session Chat 根据 `turn_end` 事实渲染失败和取消终态。
失败回合显示稳定错误码、友好文案和重试按钮；原始错误只在“查看详情”中展示。
失败回合只在聊天区内联展示，不再同时弹出顶部 banner。
重试会重发该回合的用户输入，不清空输入框草稿。
取消回合显示“本轮已取消”，并保留已经写入的助手消息。


## 消息 reasoning

message fact 可保存 `reasoning`。
`session.messages` 把它随助手消息返回。
Session Chat 默认折叠思考过程，并在 summary 中显示字符数。
思考正文首次展开时才做 Markdown 渲染；超过 32 KiB 默认只渲染前 32 KiB。
运行中的推理正文最多写入 32 KiB DOM，超出后提示“显示已截断”。
生成中的推理 summary 会附加“生成中”和已用时；长时间只推理时提示可停止后重试。

## 工具消息

message fact 可保存 `tool_calls` 和 `tool_call_id`。
Agent 用 `role=tool` 的 message Fact 保存工具结果。
工具结果的 `is_error`、`truncated` 和 `meta` 保留在 Fact 中。

## 大输出与长会话

Session Chat 对工具消息正文按 8 KiB 截断。
截断时显示省略字符数和“展开全文”按钮。
展开后才把完整正文写入 DOM。

Session Chat 默认渲染最近 80 个时间线条目。
Session Trace 默认渲染最近 200 条 Facts。
顶部“加载更早”按钮扩大窗口，并保持当前滚动位置。

## Facts 复制与导出

Session Trace 支持逐条多选 facts。
全选只作用于当前筛选结果。
复制和导出只包含已选 facts。
两者使用相同的 JSON 内容。
JSON 包含 `session_id`、`exported_at`、`fact_count` 和 `facts`。
