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

Workspace 绑定在 Session 生命周期内不可修改。`session.update` 只更新显示名称。

历史列表和打开标签按当前 Workspace 过滤。每个 Workspace 分别保留选中会话，切换 Workspace 不关闭其他 Workspace 的后台 Turn。

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

最终 assistant `message` Fact 是正文真源。LLM delta 不写入 Session。刷新后从 Fact 重建 UI。

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

`session.state` 负责重建三个 UI State。`session.stream.update` 更新临时 Stream Draft。最终 assistant `message` Fact 到达后，Session 清除 Draft。

Session Runtime 是这三个 Component 的直接驱动者。llm-openai 不直接渲染输出。

当前交互包括 Workspace 选择与添加、按 Workspace 过滤会话、会话搜索、多会话标签、按会话保留输入草稿、后台会话运行指示、底部消息输入框、流式状态、停止生成、消息和代码复制、思考过程折叠、回到最新和稳定 revision。

## 7. 文档

详细契约见 [communication.md](communication.md)。该文档也记录前端通信。
