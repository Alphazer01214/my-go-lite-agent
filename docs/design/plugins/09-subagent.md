# subagent

> 状态：当前实现（v2）。单个 `subagent` 工具、阻塞调用、子会话只读浏览已落地。
> 涉及能力：`subagent`
> 依赖：弱依赖 `session`、`tools`、`loop`

本文是 `subagent` 插件的当前契约。旧版的 spawn/wait/cancel 方案已废弃。

## 1. 职责

- 把一段自足任务交给独立子会话执行。
- 子代理的最终回答作为这次工具调用的结果返回给父代理。
- 在父会话记录子代理的开始与结束，供界面建立父子关系。
- 子会话使用父会话的工作区、模型和工具面。
- 深度上限 1：子代理不能再委派。

## 2. 非职责

- 不实现推理循环。子代理跑在 `loop` 能力上。
- 不拼装 prompt。子代理 prompt 由 model-context 管理。
- 不保存画像或 prompt 文本。插件里没有 `profiles.go`。
- 不做后台任务、续聊、跨代理消息或独立取消。
- 不走消费链。`consumes` 为空。

## 3. Manifest

```json
{
  "name": "subagent",
  "version": "0.1.0",
  "protocol": 1,
  "entry": "subagent",
  "description": "Delegate a self-contained task to a child agent and return its final answer",
  "autostart": true,
  "priority": 0,
  "provides": ["subagent"],
  "consumes": [],
  "requires": ["session", "tools", "loop"],
  "depends_on": [],
  "commands": []
}
```

## 4. 方法

### 4.1 subagent.run

模型不直接调用这个方法；它通过工具名 `subagent` 发起调用。

输入：

```json
{
  "prompt": "找出所有读取 config.yaml 的调用点。",
  "max_steps": 24
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `prompt` | 是 | 自足任务说明。子代理看不到父会话 |
| `max_steps` | 否 | 子轮次步数上限。缺省用配置值 |

输出是 Tool Result 包络：

```json
{
  "content": "子代理最终回答",
  "is_error": false,
  "truncated": false,
  "meta": {
    "child_session_id": "s-child-7",
    "state": "completed",
    "duration_ms": 4210,
    "turn_count": 3,
    "usage": {"prompt_tokens": 8120, "completion_tokens": 640, "total_tokens": 8760}
  }
}
```

子代理最终回答只通过 `content` 进入父会话一次。父会话不再额外写一条结果消息事实。

错误码：

| 场景 | 错误码 |
|---|---|
| 没有父会话 | `subagent_parent_unresolved` |
| 深度超限 | `subagent_depth_exceeded` |
| 子会话创建失败 | `subagent_spawn_failed` |
| 工具面不可读 | `subagent_tools_unavailable` |
| 子代理内部失败 | Tool Result `is_error=true`，不是协议错误 |

## 5. 接缝与调用关系

```text
model
  -> subagent tool
  -> tools.call
  -> subagent.run
  -> session.create(child: origin=subagent, parent_session_id, delegation_depth)
  -> loop.turn(child)
  -> llm.complete
  -> model-context
  -> tools
```

- 子代理使用与主代理相同的 Agent 循环。
- 父轮次的上下文取消会透传到子轮次，不需要单独的 cancel 工具。
- 子代理不能嵌套：工具面移除 `subagent` 及所有 `subagent_*` 工具。

## 6. 工具契约

| 工具名 | 目标能力 | 目标方法 | 风险 |
|---|---|---|---:|
| `subagent` | `subagent` | `run` | 20 |

工具描述包含委派语义和“Writing the prompt”元指令，和 Claude Code 的 Agent 工具同属一类。工具本身由 subagent 插件通过 `tools.sync` 注册；model-context 像其他工具一样把定义交给模型。

`subagent` 声明 `parallel_safe: true`。agent 的 `tool_parallelism=auto` 时，同一批多个 subagent 调用可以并发；父会话仍按原始 tool_calls 顺序写回工具结果。多个子代理共享 workspace，父代理应委派互不重叠的任务；并发上限由 agent 的 `max_parallel_tools` 控制。

## 7. Prompt 贡献

**父代理。**

- 工具描述告诉模型何时委派、如何写自足 prompt。
- 不往主静态 prompt 里写子代理画像。

**子代理。**

- model-context 读取 `session.info` 的 `origin`。
- `origin=subagent` 时，把 `plugins/model-context/config.json` 的 `subagent_sections` 追加到静态轨。
- 子代理插件不保存、不硬编码任何 prompt 文本。
- 初始任务作为子会话的第一条 user 消息。
- 子会话不继承父对话。

配置示例：

```json
{
  "subagent_sections": [
    {
      "name": "subagent",
      "order": 1000,
      "text": "You are a subagent delegated one self-contained task..."
    }
  ]
}
```

## 8. 记录与持久化

父会话写两条过程事实，均不带 `message`：

| 事实类型 | 时机 | 内容 |
|---|---|---|
| `subagent_spawned` | 子轮次开始前 | `child_session_id`、`state=running`、`parent_session_id`、`delegation_depth`、`started_ts` |
| `subagent_finished` | 子轮次结束后 | `child_session_id`、`state`、`duration_ms`、`turn_count`、`usage`、可选 `error` |

子会话自己的事实日志保存完整工作内容：消息、工具调用、工具结果、用量和快照。

## 9. WebUI

- 子会话默认不出现在左侧历史列表。
- 父会话聊天区显示“子代理”入口，列出 `subagent_spawned` 事实对应的子会话。
- 点击入口打开子会话，展示完整消息与工具结果。
- 子会话是只读视图：不显示输入框，不提供发送，只提供“返回父会话”。
- 子会话仍可通过 `session.info`、`session.state`、`session.derive` 直接访问。

## 10. 配置

```json
{
  "max_depth": 1,
  "default_max_steps": 24
}
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `max_depth` | 1 | 委派深度上限 |
| `default_max_steps` | 24 | 子代理默认步数上限 |

## 11. 测试要求

| 项 | 用例 |
|---|---|
| 工具调用 | `subagent` 工具存在并分发到 `(subagent, run)` |
| 子会话 | 创建时带 parent、origin=subagent、depth |
| 结果 | 子代理最终回答等于工具结果 `content` |
| 失败 | 子轮次失败返回 `is_error=true`，父会话有 finished 事实 |
| 深度 | 超过 `max_depth` 返回 `subagent_depth_exceeded` |
| 工具面 | 子代理工具面不包含 `subagent` |
| Prompt | `origin=subagent` 时 model-context 追加 `subagent_sections`；主会话不追加 |
| UI | 父会话显示子代理入口；子会话只读、无输入框 |

## 12. 未实现

- 后台子代理与并行编排。
- 子代理续聊与独立取消工具。
- 内置 explorer/implementer/verifier 画像。
- 继承父轮次额外的 allow/deny 工具限制。
- 跨代理消息与父子双向通信。
