# subagent

`subagent` 运行同步子 Agent Turn，并把结果返回给主 Agent。

## 能力

| 方法 | 用途 |
|---|---|
| `subagent.run` | 创建子 Session 并运行 Turn |
| `subagent.cancel` | 取消运行中的子任务 |
| `subagent.status` | 查询任务快照 |
| `subagent.list` | 列出当前进程内任务 |

## 工具

`subagent_run` 指向 `subagent.run`，风险为 `medium`。

子 Turn 使用 `origin=subagent`，并记录 `parent_session_id` 和 `delegation_depth`。默认最大深度为 2，最大并发为 4。

子 Turn 默认禁用 `subagent_run` 和 `subagent_cancel`，防止无限递归。

## 配置

```json
{"max_depth": 2, "max_concurrent": 4}
```

完整契约见 [communication.md](communication.md)。
