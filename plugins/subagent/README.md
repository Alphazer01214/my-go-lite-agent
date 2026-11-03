状态：当前实现

# Subagent

Subagent 把一段自足任务交给子会话执行，并把子代理的最终回答作为一次工具调用的结果返回给父代理。

## 方法

| 方法 | 说明 |
|---|---|
| `subagent.info` | 返回 `subagent` 工具定义，供 `tools` 目录拉取 |
| `subagent.run` | 创建子会话并执行一次 `loop.turn` |
| `subagent.settings.get` | 读取最大委派深度 |
| `subagent.settings.save` | 保存最大委派深度 |

`subagent` 不注册到 `tools` 之外；模型只通过工具名 `subagent` 调用它。

## 子会话

`subagent.run` 的输入来自 `tools`：

```json
{
  "arguments": { "prompt": "self-contained brief" },
  "session_id": "parent-session",
  "workspace": "E:/project",
  "call_id": "call-1"
}
```

执行顺序：

1. 读取父会话 `session.info`。
2. 沿 `parent_session_id` 链计算委派深度，超过 `max_depth` 返回 `subagent_depth_exceeded`。
3. `session.create` 创建子会话，继承父会话的 `workspace` 和 `model`，写入 `parent_session_id` 与 `origin=subagent`。
4. 向父会话写 `subagent_spawned` 事实。
5. 调用 `loop.turn` 在子会话里跑一轮完整 Agent Loop。
6. 取最后一条有文本的 assistant 消息作为最终回答。
7. 向父会话写 `subagent_finished` 事实。
8. 返回 Tool Result 包络。

子会话有独立的 Fact 日志。`prompter` 在 `session.create` 时写入与主会话相同的 system prompt / runtime context 基线，但子会话不继承父会话的消息历史。工具描述会明确提示父代理：prompt 必须自足。

子会话默认不出现在左侧历史列表；父会话的 `subagent_spawned` / `subagent_finished` 事实带有 `child_session_id`，可通过 `session.facts` 或 Trace 查看。

## 深度

`max_depth` 默认 1，即子代理不能继续委派。深度由父链计算，不依赖调用方伪造的字段。子代理仍能看到 `subagent` 工具，但嵌套调用会返回 `subagent_depth_exceeded`。

## 取消与错误

- 父 turn 取消会沿 `tool.call -> subagent.run -> loop.turn` 传播，子 turn 一并取消。
- 取消返回 `call_cancelled`。
- 子 turn 的其他失败返回 `is_error=true` 的 Tool Result，父模型可以决定重试或报告。
- 父会话不存在：`subagent_parent_unresolved`。
- 子会话创建失败：`subagent_spawn_failed`。

## 配置

```json
{
  "max_depth": 1
}
```

## 接入

`tools` 的 `config.json` 的 `capabilities` 必须包含 `subagent`，模型才会看到该工具。不要给 `tools` 增加 `depends_on: ["subagent"]`，否则会与 `subagent -> agent -> tools` 形成启动环。
