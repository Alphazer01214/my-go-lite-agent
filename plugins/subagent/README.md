# subagent

`subagent` 把一段自足任务交给独立子会话执行，子代理的最终回答就是这次工具调用的结果。

## 工具调用

模型侧只有一个工具：

| 工具 | 目标方法 | 行为 |
|---|---|---|
| `subagent` | `subagent.run` | 建子会话、跑一轮 `loop.turn`、返回最终回答 |

它和其他工具一样由 `tools.sync` 注册，经 `tools.call` 分发。调用会阻塞到子代理结束；父轮次取消时，子轮次随上下文一起取消。没有 spawn/wait 状态机，也没有后台任务。

## 子会话

子代理链路与 agent 完全相同：

```text
subagent tool -> session.create(child)
              -> loop.turn(child)
              -> llm.complete -> model-context -> tools
```

子会话的 `origin` 是 `subagent`，`parent_session_id` 与 `delegation_depth` 写在 `session_start` 中。子代理拿到父会话的工作区、模型和除 `subagent` 外的全部工具；不能再委派。

## Prompt

子代理 prompt 由 model-context 管理，不在本插件里硬编码。model-context 读取 `session.info` 的 `origin`，当 `origin=subagent` 时把 `config.json` 中的 `subagent_sections` 追加到静态轨。

任务说明作为子会话的第一条 user 消息。父会话不继承给子会话。

## 记录与界面

父会话写两条过程事实：

- `subagent_spawned`：子会话开始；
- `subagent_finished`：子会话结束、耗时、用量和错误。

父会话聊天区显示“子代理”入口；点击打开子会话。子会话以只读模式展示完整工作内容，没有输入框，只提供“返回父会话”。

## 限制

- 子代理阻塞到完成；没有后台、续聊或并行编排。
- 子代理工具面是全局可用工具去掉委派工具，不继承父轮次额外的 allow/deny。
- 深度上限为 1；子会话不能再委派。
- 取消沿父轮次上下文传播，没有独立的 cancel 工具。
