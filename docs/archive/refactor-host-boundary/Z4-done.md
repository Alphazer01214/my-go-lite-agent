# Z4 进展票据 — 拆 serve 组合层（已完成）

状态：Z4 完成，serve/serve.go 277 行（≤300），arch-check 12/12 不回退，
go build / go vet / go test ./... 全绿。

## Commit

1. `(本 Zone commit)` refactor(serve): Z4 拆 serve 为六域顶层文件，serve.go 277 行组合层
   - 不回退任何检查项；serve.go 从 1911 行降到 277 行
   - 新增 transport.go / registry.go / router.go / session.go / context.go / medium.go

## 执行取舍：spec「六包」→ 六域顶层文件（显式标注）

spec Z4 按 transport / registry / router / session / context / medium 六包切分。
本 Zone 未建独立子包，改为 serve 包内六域顶层文件 + serve.go 组合层。原因：

1. **C4 允许清单约束**：独立子包必须导出跨包访问器，会立刻爆破 ADR-0016 的
   22 方法 allowlist（C4 检查 serve/*.go 顶层 `func (s *Server)`，子包内方法
   虽不匹配正则，但跨包调用要求 Server 内部状态对子包可见，只能靠导出）。
2. **C5 / C7 位置要求**：`func Start(` 必须在 serve/serve.go（C5 按文件读），
   `reconcileConsumes` 与 `delete(s.degraded` 字面量须在 serve/*.go（C7 顶层
   文本匹配，递归子包也匹配但无必要）。
3. **共享状态单一真源**：Server 字段（provides/toolOwners/pending/turnStates…）
   被六域共享，同包文件切分保持单一状态面，不需要引入访问器层。
4. 行为零变化：纯机械搬移，函数体逐字保留，未改任何路由/注册/会话语义。

## 文件归属（final）

| 文件 | 内容 |
|------|------|
| serve.go (277) | 组合层：Server 结构、Start、SetCatalog/SetPluginsDir/MountedPluginNames/DegradedNames/ToolOwners、CallByCap、CallByFace、MarshalPayload、能力契约常量别名、LLM 方法常量 |
| transport.go | wait/waitKind/CallResult/proc、ensureCatalog/launch/readLoop/markUnhealthy/ensureAlive/handleFromPlugin/writeTo/call/callByPlugin/callStream/callStreamOn/callOnce、Close |
| registry.go | RegistrySnapshot/Registry/registry、registerProvides/discoverTools/toolsListMerged/toolsOwnerFor/reconcileConsumes/containsString |
| router.go | routeRequest/routeToolsFromPlugin/complete/collectEvent/dispatchRender/dispatchPanel/validatePanelOp/rejectPanel/handleEnsurePlugins/EnsurePlugins/handleAgentFromPlugin |
| session.go | sessionTurn/normalizeSessionID/turnFor/acquireTurn/beginRunning/endRunning/isRunning/IsRunningOn/RunningSessions/StatusForSession/emitStatus/RunTurn/RunTurnOn/CancelTurnOn/TurnCancelledOn/runTurn/runExternalTurn/messagesEqual/bytesEqualJSON + Message/ToolCall/TurnResult |
| context.go | callByCapOwner/agentDerive/agentAppend/AgentRequest/AgentInject/defaultSystemRole + AgentRequestResult |
| medium.go | presentation/commands/UI 常量、PresentationCard 别名、Cards/recordCard（medium 域 = Event/Subscriber/panels/cards，spec 原文） |

注：Close 归 transport.go（spec 原文：transport = proc/readLoop/pending/超时/Close/ensureAlive）。
Presentation/Commands 常量、Cards 归 medium.go 是 spec「medium（Event/Subscriber/panels/cards）」
的直接依据，非 serve.go 组合层内容。

## 校验

- `gofmt -l` 新文件与服务端 serve.go 干净；debug.go/events.go/render.go 及其
  测试文件为先前提交已存在的不格式化，不在本 Zone 改动面，未触碰。
- `go vet ./...` 0 错误；`go test ./...` 全绿（serve 8.9s / web 8.0s 集成测试）。
- arch-check：12/12（C4/C5/C7 均在新布局下通过）。
