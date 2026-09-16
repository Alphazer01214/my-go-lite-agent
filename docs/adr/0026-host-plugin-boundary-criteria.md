# Host ↔ Plugin 边界判据

**Status: accepted**

Host 只提供通用方法，不对任何插件做特化。为让「特化」可机械判定（而非靠直觉），按下表划分知识边界：

| 层 | 内容 | Host |
|----|------|------|
| L0 通用路由原语 | Frame 转发与 `fwd-id`、pending 表、超时、进程生命周期、`host.*` 横切面、发送原语 | 应拥有 |
| L1 公开契约能力名 | `session` `llm` `loop` `tools` `context` `system-prompt` `policy` `skills` `ui` `commands` `presentation` 及各自方法 | 可作为消费者调用；名字是公开契约，任何插件均可实现 |
| L2 插件私有 | 插件名字面量、payload 字段语义、业务决策、展示规则 | 绝不能知道 |

三条可机械检查的规则：

1. **不认识插件名**——`serve/` `web/` `internal/` 不得出现 `plugins/` 下任何目录名的字面量。
2. **不解读 payload**——不得为某能力的 payload 定义字段级结构体；Host 只透传 `json.RawMessage`。
3. **不代插件决策**——展示事件由插件自己 Emit；Host 不持有 render kind 白名单、不替插件广播业务事实。

**反例（本次审查实测，共 9 类）：** 写死 `"agent"`（scheme.go:13,27 · web/server.go:873,1070）与 `"echo"`（app.go:179,215）；`/refresh` 遍历插件名发 `config.reload` 并吞错误（commands.go:63-76）；`AppendSessionFacts` 自建 `Topic:"session"` 事件（serve.go:1486-1494）；`extractStreamDelta` 解析 `llm.chunk` 与 `presentation.stream` 字段（serve.go:1719-1739）；`complete()` 按 `session.append`/`llm.complete` 挂副作用（serve.go:1005-1016）；14 个 wrapper 把插件方法名升为 Host 公开 API；`handleWorkspaceResolve` 在 Host 做目录遍历（web/server.go:440-524）；`validatePanelOp` 内建 UI 契约（serve.go:720-748，此项由 ADR-0010 授权，保留）；Plugin Graph 的 `system-prompt` 假边（web/server.go:800）。

**正例（已是此形态，勿动）：** `policy` / `skills` / `project-context`——Host 只在 Plugin Graph 里画节点，从不调用、从不解读。

代价：Host 无法为某个插件做便捷适配，Medium 须直接对齐公开能力契约。收益：替换任一插件不需要改 Host，与 ADR-0016「一切皆插件」同源。
