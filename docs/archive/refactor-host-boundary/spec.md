# refactor-host-boundary：Host 去特化与注册真源统一

Status: resolved

## Intent

把 Host 从「认识具体插件」改造为「只认识公开能力契约」，同时修复审查确认的缺陷。
完成判据是机械的：`scripts/arch-check.sh` 12/12 翻绿。决策已经裁决并落进
ADR-0026..0029，本 spec 只负责把执行顺序、任务坐标与约束写清楚。

## 必读文档（按顺序）

| 序 | 文档 | 作用 |
|----|------|------|
| 1 | `docs/adr/0026-host-plugin-boundary-criteria.md` | 判据：L0/L1/L2 + 三条机械规则，C1/C2/C3/C10 的依据 |
| 2 | `docs/adr/0027-plugin-host-faces.md` | hostFaces 转正、agent-presets、`-scheme` 删除、protocol 4 |
| 3 | `docs/adr/0028-auto-compact-as-default.md` | 自动压缩已是默认行为——勿当 bug 修 |
| 4 | `docs/adr/0029-unified-medium-logic.md` | 单槽 `On*` 钩子改订阅列表，C11 的依据 |
| 5 | `docs/adr/0016` | Host 允许保留的面，C4 允许清单的依据 |
| 6 | `CONTEXT.md` | 术语真源（含新增 Sandbox / Tool Severity），输出用规范名词 |
| 7 | `scripts/arch-check.sh` | 验收标准本体：12 项检查，全绿即完成 |
| 8 | `docs/agents/domain.md` | 约定：术语用规范名词；ADR 冲突显式标注、不静默覆盖 |
| 9 | `docs/agents/issue-tracker.md` | 本目录的票据约定 |

背景（可选，供理解「为什么」）：`.workbuddy/deliverables/` 下四份审查报告；
需求意图见 `.scratch/autostart-depends-scheme/spec.md`、`.scratch/phase1-coding-agent/spec.md`、
`.scratch/plugin-config/spec.md`；用户面行为基准见 `README.md` 与 `plugins/README.md`
（重构后行为不得偏离，除非对应 ADR 授权）。

## 全程标准

- 每个 Zone 结束：`go build ./... && go vet ./... && go test ./...` 全绿。
- `bash scripts/arch-check.sh`：已绿项不回退，本 Zone 目标项翻绿。
- 一个逻辑单元一个 commit；message 写明翻绿了哪个检查项；禁止 `--no-verify`。
- Zone 顺序固定 Z1→Z5：先清理、再统一注册真源、再去特化、最后拆包。
- 测试是进程边界集成测试（现场 `go build` 夹具插件），慢是正常的，不要为提速改成 mock。
- 允许破坏性变更（已授权），但每处破坏必须能在对应 ADR 里找到出处。

## Zone 任务表

### Z1 零风险清理（不翻绿任何检查，为后续减负）

- 删 `deadcode` 判定的死代码：`assembly.autostart.ExpandDepends`、
  `serve.Server.checkConsumes`、`serve.Server.emitRenderIntent`、
  `serve.render.JSONToPairs/compactJSON/formatRunningLine`、
  `serve.DebugEnabled`、`web.Server.ListenAndServe`、`mdansi.Plain`、
  `agent.mapToToolSchema/isReadOnlySchema`、`agent.activeSchemeName`、
  `serve.Server.NewSessionID`。
  **不删** `pluginsdk` 的 4 个 emitter（公开 SDK，Z3 会接线）。
- 依赖闭包算法三份收敛一份：`ResolveAutostart` 的 `add` 递归保留，
  `EnsurePlugins` 的 `walk` 改为调用它。
- 8 个无 manifest 夹具目录（agentprobe asyncsubllm consumer crashonce
  emptytools promptreg sessionprobe slow）移 `testdata/plugins/`，
  同步改全部测试引用与 `scripts/build.{sh,ps1}` 的排除清单。
- 删 `examples/*.json`（5 个）与 `internal/app/app.go` 的 legacy
  `assembly.Resolve` 分支（ADR-0021；`assembly` 包与其测试保留）。
- 12 个已产出 ADR 的 `.scratch` 目录归档 `docs/archive/`，只留未完结的。
- 18 个 `ready-for-agent` issue 完整对账后翻 `resolved`（对账，勿凭猜测）。
- `sdk/lite-agent.js` 副本改 `go:generate`；两个 build 脚本共享一份插件清单。

### Z2 统一注册真源（翻绿 C2 / C3 / C7 / C8）

- `plugin/manifest.go` 加 `hostFaces`（config|commands|ui），`CurrentProtocol` 升 4。
- `registerProvides` 按 hostFaces 派发；`/refresh` 只广播给声明 `config`
  面的插件，删除吞错误的循环。
- `softCheckConsumes` → `reconcileConsumes()`：局部构建 + 一次性 swap，
  幂等、可清除 degraded、可恢复 provides，加世代计数供 `/api/plugins` 显示。
- `discoverTools` 改「全量成功才替换」，重名策略与 `toolsListMerged` 统一；
  插件重启后重跑，随后删 `routeToolsFromPlugin` 的试探 fallback。
- `plugins/agent` 的 `consumes` 补全（system-prompt/context/session/llm/tools）。
  **修正标注（fixup，2026-09）**：`tools` 已从 consumes 移除——autostart 集
  无 tools 提供方（filetools 等按 scheme dependsPlugins 拉起），reconcile
  （ADR-0022）会默认把 agent 判 degraded 并撤回 loop，使 `-turn` 开箱即坏；
  agent 对无 tools 有显式降级路径（toolsOK=false 提示继续纯聊天），tools 属
  可选依赖，consumes 只留硬依赖。见 fixup-agent-consumes.md。
- `web/server.go` 的 `hostUsedCapabilities` 删 `SystemPromptCap` 假边；
  Plugin Graph 改读注册表快照并显示「声明 vs 实际」差异。
- 新增 `agent-presets` 能力（agent 插件可选实现），Plugin Graph 的 scheme
  边改由 `CallByCap("agent-presets","get")` 取数。

### Z3 Host 去特化（翻绿 C1 / C4 / C6 / C10 / C11 / C12）

- 删 `-scheme` flag（cli.go / server.go）与 `internal/app/scheme.go`；
  scheme 切换改由 agent 的 `/agent scheme <name>` 命令承载。
- 删 `app.go` 探针的 `echo` 能力默认值（要求显式 `-frame-cap`/`-frame-method`）。
- 删 `publishPluginSessionAppend` 与 `AppendSessionFacts` 内的自建事件；
  session 插件改为自行 Emit。
- 删 `dispatchRender` 的 render kind 白名单（原样转发，Medium 忽略未知 kind）。
- 删 `extractStreamDelta`（Host 不再解析 llm/presentation 私有字段）。
- 删 `complete()` 里按 `session.append`/`llm.complete` 的两个副作用钩子
  （usage 记账改为 llm 插件显式调用 `context.noteUsage`）。
- 22 个超出 ADR-0016 允许清单的导出方法收口：内部化，或由 Medium 改走
  `CallByCap`；`Call/CallByPlugin/CallCommand/CallUIAction/CallStream*`
  收为 Host 内部原语。
- `CancelTurnOn` 改发 `TypeEvt`（或登记 pending 收 res）。
- `web/server.go` 的 `handleWorkspaceResolve` 下放为 `workspace.resolve`
  能力（filetools 或独立 workspace 插件实现）。
- `validatePanelOp` 收窄为 JSON 结构校验（组件前缀校验由 ADR-0010 授权，
  保留但注明出处）。
- `On*` 单槽字段（OnStreamDelta/OnStatus/OnToolCall/OnRender/OnToolApproval）
  改订阅列表；`wireRenderer` 的保存-还原同步改造。
- `chat` scheme 的 `dependsPlugins` 补 `filetools`；加启动自检
  `allowedTools ⊆ ⋃toolNames(dependsPlugins)`。
- 5 处插件手写 `Emit("presentation",…)` 改用 `pluginsdk` typed emitter。
- `serve` 的 4 组重复 wire 类型改为 import `pluginsdk`。

### Z4 拆 serve（2245 行单文件 → 组合层）

按 `transport`（proc/readLoop/pending/超时/Close/ensureAlive）、
`registry`（provides/toolsProviders/toolOwners/reconcile）、
`router`（routeRequest/星型/fwd-id/complete/按名与按能力两条路径）、
`session`（turnStates/Run*/Cancel*/ListSessions/CreateSession*）、
`context`（QuerySessionFacts/DeriveMessages/ContextUsage/AgentRequest）、
`medium`（Event/Subscriber/panels/cards）六包切分；
`serve/serve.go` 降到 300 行以内的组合层。
**前置：Z3 必须先完成**，否则每个新包都要暴露别人用不到的方法。

### Z5 文档收尾

- ADR-0018 正文修订（todo provider 已改为 agent 内建）。
- README 的 `chat` scheme 描述与新行为对齐。
- README 快速开始中 4 处 `-scheme` 示例全部改写（ADR-0027 已定案彻底删 flag：
  一次性 `-turn` 无法预选 scheme，改为先设 defaultScheme 或进 REPL 切换）。
- `plugin-config/spec.md` 状态更新。
- 剩余 issue 状态翻新。

## 硬约束（违反任何一条即停手汇报）

- Host 代码不得新增对 `plugins/` 下任何目录名的引用（**含注释**）。
- 不引入 cordis 的 waterfall / `internal/*` 事件分发与 symbol isolate
  （`plugin-host-runtime/spec.md` 明确不移植；ADR-0014 已删 Waterfall）。
- 保留 ADR-0010 授权的 PanelOp 组件前缀校验。
- 保留 `startMounted` 用 `os.Getwd()` 算默认 workspace（Session 初值来源，
  与 `handleWorkspaceResolve` 的越权不同类）。
- 不把插件改回同进程加载。
- 与 ADR 冲突或与本 spec 矛盾的发现：停下、显式标注、询问，不静默覆盖。

## 完成判据

- `bash scripts/arch-check.sh` 退出码 0（12/12）。
- `go test ./...` 全绿。
- 四篇 ADR（0026..0029）与代码行为一致，无未标注的偏差。
