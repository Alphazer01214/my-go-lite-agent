# session

`session` 是 Session Fact Log 的唯一业务真源。它保存 append-only 历史，并提供查询和模型上下文投影。Session Runtime 还负责驱动 Session WebUI。

## 1. 能力

| Capability | 说明 |
|---|---|
| `session` | Session 生命周期、Fact、Workspace Registry 和投影 |

Session 不调用 LLM，也不执行工具。

## 2. Workspace Registry

Session 插件自身拥有 Workspace Registry。注册记录保存在：

```text
plugins/session/sessions/workspaces.json
```

它负责目录规范化、当前选择、重命名和移除。Workspace 不通过独立 Capability 暴露。

Session 属于一个 Workspace。创建时传入 `workspace_id`，Session 将它和规范路径写入 `session_start` Fact。

Workspace 绑定在 Session 生命周期内不可修改。`session.update` 更新显示名称与置顶状态。
`session.delete` 删除会话日志；有未闭合 Turn 时拒绝。置顶会话排在历史列表前面，状态写入 `session_meta`，重启后恢复。

创建时还可以传入 `model` 和 `context_window`，它们同样写入 `session_start`，创建后不变。Session 不校验模型目录，
只保存调用方选择的模型 id 和窗口。

历史列表和打开标签按当前 Workspace 过滤。每个 Workspace 分别保留选中会话，切换 Workspace 不关闭其他 Workspace 的后台 Turn。
切换 Workspace 时优先恢复该 Workspace 的选中会话；没有记录时选中最新会话；没有会话时清空选中。

## 3. 持久化

一个 Session 对应一个 JSONL 文件：

```text
plugins/session/sessions/<session_id>.jsonl
```

每行一个 Fact。`seq` 在 Session 内单调递增。损坏行读取时跳过并写警告。

`sessions/workspaces.json` 保存 Workspace Registry。它会随整个 `sessions/` 目录在构建时保留。

`SESSION_WORKSPACES_FILE` 可覆盖 Registry 路径。`LEGACY_WORKSPACE_DATA_FILE` 可覆盖旧版 Registry 的来源路径。

首次启动时，如果旧版 `<数据根>/workspace/workspaces.json` 存在，Session 会把它迁移到自己的数据目录。
旧路径是从 Session 目录向上两级推导的（`<数据根>/session/sessions` → `<数据根>/workspace`），
不匹配这个布局时找不到就是找不到，不会去猜别的位置；需要时用 `LEGACY_WORKSPACE_DATA_FILE` 显式指定。
迁移写盘失败会直接报错并中止启动，不会静默丢弃已有 Workspace 列表。

迁移只发生在启动时。`session.info` 和 `session.state` 是读接口，不写日志也不写 Registry。

## 4. Fact 真源

Fact Log 只追加，不更新、不删除。最终 assistant `message` Fact 是正文真源。LLM delta 不写入 Session。刷新后从 Fact 重建 UI。

model-context 会把渲染后的 system prompt、runtime context、压缩摘要和请求快照写成 Facts。`session.derive` 把这些内容投影为模型可见消息：

- 最新 system prompt 放到头部；
- 最新 active 摘要用 shadow range 遮蔽旧消息；
- 最新 runtime context 放在摘要之后；
- 旧 tool result 按配置替换为确定性 stub；
- 原始消息始终保留在日志中。

压缩只追加 `context_summary` 与 `context_compaction`，不删除旧消息。`context_snapshot` 记录某一步实际发送的 surface 和 hash。

## 5. 配置

当前配置：

```json
{
  "fullToolResults": 8
}
```

该值控制 `session.derive` 保留完整结果的最近工具消息数量。更早的工具结果替换为摘要。

## 6. WebUI

Session 拥有：

| Component | Slot | 职责 |
|---|---|---|
| `session-history` | left | 会话列表和元数据 |
| `session-chat` | center | 消息、Turn 和 Stream Draft |
| `session-trace` | right | Fact Trace |
| `session-dock` | bottom | 模型、API usage、上下文占用和 Turn 状态 |

`session.state` 负责重建前三个 UI State，`session.dock` 负责重建底部 Dock State。`session.stream.update` 更新临时 Stream Draft。
最终 assistant `message` Fact 到达后，Session 清除 Draft。

`session-dock` 的上下文百分比使用最近一次 `llm_usage` 的 `prompt_tokens` 除以模型窗口；会话累计用量同样来自 API 返回值。
`context_snapshot` 的 `token_estimate` 只在明细面板中标注为估算。面板通过 `model-context.inspect` 展开实际发送的消息，
通过 `model-context.build` 预览下一次请求。没有选中会话时 dock 隐藏，Host 的 bottom Slot 也随之隐藏。

Session Runtime 是这些 Component 的直接驱动者。llm-openai 不直接渲染输出。

当前交互包括 Workspace 选择与添加、按 Workspace 过滤会话、会话搜索、会话改名/置顶/删除、多会话标签、按会话保留输入草稿、后台会话运行指示、底部消息输入框、流式状态、停止生成、消息和代码复制、思考过程折叠、回到最新和稳定 revision。

会话操作在历史列表行右侧：改名打开对话框，删除需要连续点击两次确认。运行中的会话由后端拒绝删除。

聊天消息按 Turn 折叠：完成轮默认收起过程，只保留用户消息、上下文块和最终回答；Turn 头显示状态、过程条数、工具数和时长。
没有过程行的轮次显示状态行，零工具完成的轮次标注「未调用工具」。展开后，工具结果、reasoning 和 tool_calls 仍各自可折叠。
折叠选择只存在浏览器内存中。

左侧“新建会话”旁有模型下拉，通过 `llm.models.list` 读取模型目录；模型目录的编辑在插件中心 → 设置 → `llm-openai` 中完成。
创建会话时选中的 `model/context_window` 写入 `session_start`。设置保存成功后 Host 派发 `liteagent:settings-saved`，
下拉会重新读取模型目录。llm 能力不可用时该行隐藏，会话仍按 llm 的默认模型运行。

## 7. 文档

详细契约见 [communication.md](communication.md)。该文档也记录前端通信。
