# l0-only-boundary：Host 与 Medium 领域清零

Status: resolved（#3 agent.request|inject|confirm 仍 Deferred，另开票据）

共识见 [decisions.md](./decisions.md)（**实现面已定；#3 request/inject/confirm 暂缓**）。  
**已定并落地**：启动只带 plugins；Host 按插件名转发（Frame `to`+payload，protocol 5）；static=Medium 且流式在插件 UI；金路径插件化；arch-check.sh 已删。  
**暂缓**：agent.request|inject|confirm —— Host 临时保留特例。

## Intent

落实 ADR-0030：Host/Medium（含 static）领域出核；Host 只认识插件并按名转发；领域与流式会话 UI 归插件。

完成判据：`go build && go vet && go test ./...` 全绿 + 核四件金路径集成测试；**无 arch-check 脚本**。

## 必读

| 序 | 文档 |
|----|------|
| 1 | `docs/adr/0030-l0-only-host-and-medium.md` |
| 2 | `decisions.md` |
| 3 | `docs/adr/0026` / `0016` / `0002` / `0027` / `0023` / `0010` / `0021` |
| 4 | `CONTEXT.md`、`plugins/README.md`、`README.md` |

## 评审清单（取代原 C13–C15 脚本；人工/测试约束）

- `serve/` 无 `RunTurn`/`AgentRequest`/`AgentInject`/Message/TurnResult；无 cap→业务 switch
- `serve/` 无 tools.list 合并与按名 tool 路由
- `web/` 无 `/api/session|turn|message|tool-approval|workspace`
- `web/static/` 无 sendMessage、session/tool_approval 业务流；无聊天流状态机
- `internal/app/` 无领域启动 flag；session_agent 领域编排已拆
- 启动 argv 仅 L0
- 金路径：核四件装配后 Web 可聊、CLI REPL 可聊

Z0 删除 `scripts/arch-check.sh`。

## 全程标准

- 每 Zone：`go build ./... && go vet ./... && go test ./...` 全绿。
- 一个逻辑单元一个 commit；禁止 `--no-verify`。
- 允许破坏性变更（ADR-0030 授权），每处破坏写明出处。
- 与 ADR 冲突：停下、标注、询问，不静默覆盖。
- 测试仍以进程边界集成测试为主，不为提速改成 mock。

## Zone 任务表

### Z0 基线与术语

- 删除 `scripts/arch-check.sh` 及文档中「arch-check 12/12」引用。
- ADR-0030 Status → accepted（用户确认后）。
- CONTEXT.md：Host 词条改为「按插件名转发、不认识能力名」；横切面词条待命名定案后补。
- 待议项关闭后再动 Z2 协议面。

### Z1 契约搬家

- `Message`/`ToolCall`/`TurnResult`/流式 payload → pluginsdk。
- Frame v5 寻址信封定稿（见 decisions「路由模型细化」）。

### Z2 Host 清零 + 点名路由

- 删除 cap 注册表路由；Frame/`/api/call` 改 `to` + 不透明 payload；pending/超时/fwd-id 保留。
- 删除 tools 合并、`toolOwners`、`routeToolsFromPlugin` 领域逻辑。
- 删除 RunTurn/turnStates/能力别名/Message/TurnResult（进 pluginsdk 后）。
- 不变量 → session 插件 `validate`；锁/cancel → loop 插件。
- **`agent.request|inject|confirm`：暂缓，临时保留**（唯一业务名分支），直到 #3 重开。
- `CurrentProtocol = 5`；出厂 manifest 升级。

### Z3 Web Medium 清零（含 static）

- 删领域 HTTP 面与 `currentSession`；`/api/call` 改为 `to`+payload。
- Shell：删 `sendMessage`、session/tool_approval 业务监听；**流式与会话状态机进插件 `ui/`**。
- sdk.js 收 L0 桥；evt 不透明透传给已装载组件。
- Layout role 数据（如 session-view）允许；Shell 不对 role 做业务分支。

### Z4 CLI Medium 清零

- 启动 argv 只留 `-plugins` `-repl` `-serve` `-debug` `-discover` `-dump`。
- 删 `-turn` `-session-*` `-agent-*` `-context-list` `-cards` `-workspace`。
- `session_agent.go` 领域编排拆除；`turnRenderer` 用 pluginsdk 绘制；审批改通用 ask。
- 通用 `-invoke`/`-frame-*`/`-plugin` 归 `invoke.go`。
- REPL：非 slash 输入由插件 commands 承接。

### Z5 金路径与文档

- session/agent/sandbox 补齐 UI（含流式）。
- 集成测试：autostart 核四件后 Web/REPL 可聊。
- Z3/Z5 审批：先落通用 ask **骨架**；展示细节等 #3。
- 本 spec Status → resolved（#3 另开票据，不阻塞本 spec 关闭，但 ADR-0030 保持 Deferred 注记）。

## 硬约束

- 不把插件改回同进程；不恢复 waterfall。
- 保留 host.ensurePlugins、hostFaces、PanelOp 前缀校验。
- Host/Medium 不写死插件目录名做调度分支。
- 与 ADR 冲突：停下、标注、询问。

## 完成判据

- `go test ./...` 全绿；金路径集成测试通过。
- 无 arch-check.sh。
- ADR-0030 与代码一致；待议项已定案并落地。

## 风险

1. 不变量离开星型中心后可被绕过 → 金路径 + agent 文档约束。
2. Medium/static 收缩是产品级破坏；README/测试大修。
3. 无 cap 注册表后插件点名依赖 scheme/dependsOn/发现数据——默认装配仍靠 autostart。
4. 流式 UI 迁插件是 Web 最大工作量，Z3/Z5 要一起排期。
