# 通用 choice：session 插件拥有的问答原语

**Status: accepted**

## 背景

工具审批（`tool_approval`，ADR-0029）是一套专用机制：sandbox 发 `agent.confirm` Frame → Host 拦截器（`handleAgentFromPlugin`）→ `serve.askApproval` 并行扇出到注册的 Medium 面（CLI prompt / Web）→ Web 用自有 SSE topic + 专用端点 `/api/tool-approval` + `apprWait` 配对闭环。三个问题：

1. **域逻辑在 Host**：审批链是 agent 域的（tool/arguments），却由 Host 拦截、扇出、配对——违背 L0（ADR-0030「Host 不出现域能力分支」），Deferred 例外被永久化。
2. **不通用**：写死了 allow/deny 二元语义与 `tool/arguments` payload，未来的选项卡、问卷等任何「让用户做选择」的场景都无法复用。
3. **三处专用代码**：serve 审批面注册表、web 审批配对、CLI 终端面，各有一份等待/配对逻辑。

## 决策

**choice 是 session 插件的一个普通能力**（`provides: ["choice"]`），方法不在 Host 的任何地方出现：

| 面 | 契约 |
|---|---|
| `choice.ask`（插件 → session，Frame req，**阻塞**） | req：`{kind, prompt, options:[{value,label,danger}], sessionId, meta, timeoutMs?}`；res：`{value, reason: user\|timeout}` |
| `choice.respond`（Medium → session，经 `/api/call {to:"session",cap:"choice",method:"respond"}`） | req：`{id, value}`；res：`{ok:true}`；重复/未知 id → `unknown_choice` |
| 事件 `choice.ask` / `choice.resolved`（session Emit，无 id evt） | 经 Host **通用 evt 中继**广播；payload：`{id, kind, prompt, options, sessionId, meta, timeoutMs}` / `{id, value, reason}` |

要点：

- **触发方点名 session**：sandbox 的 policy.ask 分支调 `CallTo("session","choice","ask",…)`（kind=`tool_approval`，options allow/deny，meta 带 tool/arguments/workspace）。阻塞即「会话暂停」——turn 天然停在调用链上。只有显式 `value=="allow"` 放行；超时、错误、其他值一律 deny（fail-closed 保持）。
- **等待窗口**：session 内部默认 20s，上限 25s（必须短于外层 Frame call 超时 30s，否则结构化 `{reason:"timeout"}` 会被外层超时吞掉——沿用旧 `approvalWait < DefaultCallTimeout` 不变量）。
- **通用 evt 中继**（Host 唯一新增，~4 行）：`collectEvent` 对**无 id 且非 presentation** 的插件 evt 按 `Topic:"evt"` 透传 `{cap, method, payload}`。Host 不解释内容；这激活了 events.js 早已预留的 `evt` 监听，也是未来任何插件 UI 推送的通用通道。id 标注的 evt 仍归属所属 call，不广播。
- **先应答者生效**：session 的 `choiceCenter`（pending map + chan）保证重复/迟到的 respond 得到 `unknown_choice`；resolved 事件让所有 Medium 撤卡，replay 重连不留僵尸卡。
- **前端**：session UI 在聊天流内渲染选择卡（`tool_approval` kind 保留原审批卡样式与跨会话徽标），应答走 `LiteAgent.callCap("session","choice","respond",…)`——`/api/call` 无方法白名单，零 Medium 改动。

## 否决的备选

- **Host L0 方法**（`choice.ask`/`choice.respond` 挂 cap=host）：链路更短、Medium 零成本，但 Host 长出一截「问答」原语与两处镜像 switch，违背「Host 只保留基础设施」的方向。
- ** choice 归 agent 或 sandbox**：审批发起方恰是它们自身，路由层禁止自点名调用；且把交互能力耦合进域插件，未来问卷也得依赖它。
- **Panel 通道**（零 Host diff）：卡片只能落在面板槽位而非聊天流，跨会话徽标与撤卡都要绕行；且问卷类交互需要语义化事件而非 UI 面操作。

## 后果

- 删除：`serve.RegisterApproval`/`askApproval`/`approvals`、web `apprWait`/`requestToolApproval`/`handleToolApproval`/`/api/tool-approval`、`agent.confirm` Host 拦截器、`serve/context.go`（`AgentRequest`/`AgentInject`）、CLI 终端审批面。
- **行为变化**：仅终端（无 Web Medium）场景 ask 无人应答 → 20s 超时 deny（fail-closed）；CLI 未来可作为另一个应答方（prompt → `choice.respond`）恢复，不在本 ADR 范围。
- agent 的 `agent.request`/`agent.inject` 自点名死路径改为直连 `session.derive`/`session.append`。
- supersedes ADR-0029 中「审批面注册 + Medium 自有回程」的实现部分；**先应答者生效、超时即 deny** 的竞态规则原样继承。
