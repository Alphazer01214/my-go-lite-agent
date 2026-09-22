# Project Map — INDEX（符号速查，server + plugins）

> 路径相对仓库根；`*_test.go` 未列入。行号为当前工作区状态（未提交改动已计入）。
> 图编号对应 [DEPENDENCY.md](DEPENDENCY.md)：图1=包依赖、图2=serve 内部、图3=Web Medium、图4=插件调用图、图5=Turn 时序。

## cmd / internal/app

| 符号 | Kind | 职责摘要 | 位置 | 图 |
|---|---|---|---|---|
| `main` | fn | 薄入口，调 app.Server | cmd/liteagent-server/main.go:8 | 1 |
| `Server` | fn | server flag 解析与派发（-plugins/-serve/-layout/-debug 必需校验） | internal/app/server.go:12 | 1,5 |
| `runWeb` | fn | 启动主线：装配→布局→挂载→建会话→命令面→Web | internal/app/web.go:26 | 1,3,4 |
| `webCommandPlane` | struct | 命令面的 Web 适配（HandleOut/Complete） | internal/app/web.go:16 | 3 |
| `resolveAssembly` | fn | Scan+ResolveAutostart，软失败打印 | internal/app/app.go:29 | 1 |
| `startMounted` | fn | 开关过滤 → serve.Start → 注入 catalog/pluginsDir/disabled | internal/app/app.go:51 | 1 |
| `probeCommandFaces` | fn | 声明 commands 却无 handler 的告警探测 | internal/app/app.go:86 | — |
| `callPlugin` | fn | Host 侧点名词用包装 | internal/app/callx.go:10 | 1 |
| `commandPlane` | struct | 原生 slash 路由（/help /lp /refresh + 插件命令） | internal/app/commands.go:17 | 3 |
| `newCommandPlane` | fn | 从 Plan 构建命令面 | internal/app/commands.go:24 | 1 |
| `handleOut` | fn | slash 行 → 输出（Web /api/command 后端） | internal/app/commands.go:92 | 3 |
| `completeSlash` | fn | slash 补全候选（Web Complete 后端） | internal/app/commands.go:291 | — |
| `refresh` | fn | 重扫 manifest + 对 config face 广播 config.reload | internal/app/commands.go:37 | — |
| `agentSchemeLabel` | fn | 经 agent-presets 能力读当前 scheme（/lp 横幅） | internal/app/commands.go:263 | — |

## serve（宿主内核）

| 符号 | Kind | 职责摘要 | 位置 | 图 |
|---|---|---|---|---|
| `Server` | struct | 进程表/provides/pending/subs/disabled 等全部宿主状态 | serve/serve.go:23 | 2,4 |
| `Start` | fn | 拉起全部挂载插件（软失败），建注册表 | serve/serve.go:105 | 1,2 |
| `SetCatalog` / `SetPluginsDir` / `SetDisabledSet` | fn | 启动注入（catalog、重扫根、开关） | serve/serve.go:55/63；plugin_switch.go:78 | 1 |
| `MountedPluginNames` / `DegradedNames` | fn | 观测面（/lp、/api/plugins） | serve/serve.go:72/89 | — |
| `CallByFace` | fn | hostFace 校验后调用（config/commands/ui） | serve/serve.go:166 | 2,3 |
| `CallByPlugin` | fn | L0 按名调用 | serve/serve.go:205 | 2,3 |
| `HostCap` | const | "host"：宿主横切面方法名 | serve/serve.go:52 | 4 |
| `proc` | struct | 插件子进程句柄（stdin/healthy/timeout/gen） | serve/transport.go:42 | 2 |
| `wait` / `CallResult` | struct | pending 调用与结果（含 evt 采集） | serve/transport.go:23/37 | 2 |
| `launch` | fn | exec 插件、Job Object 绑定、代际号、读循环 | serve/transport.go:69 | 2 |
| `readLoop` | fn | 逐帧读子进程输出 | serve/transport.go:124 | 2 |
| `markUnhealthy` | fn | 进程死亡：fail pending + 重算注册表 | serve/transport.go:137 | 2 |
| `ensureAlive` | fn | 按需重启（崩溃恢复） | serve/transport.go:185 | 2 |
| `handleFromPlugin` | fn | req/res/evt 三分派 | serve/transport.go:224 | 2 |
| `writeTo` | fn | 向插件写帧（写锁） | serve/transport.go:235 | 2 |
| `call` / `callByPlugin` / `callStream` / `callStreamOn` | fn | Host 调用族（plugin_down 重试一次） | serve/transport.go:253/264/286/292 | 2,5 |
| `callOnce` | fn | 单次调用：登记 host-id pending + 超时 | serve/transport.go:372 | 2,5 |
| `Close` | fn | EOF→宽限→killTree→Job 收尾 | serve/transport.go:320 | — |
| `routeRequest` | fn | host.* 分派 + To 点名转发（ADR-0030） | serve/router.go:20 | 2 |
| `forwardTo` | fn | fwd-id 改写、pending、超时 | serve/router.go:76 | 2 |
| `complete` | fn | res 回包还原原 id；plugin 调用无副作用 | serve/router.go:143 | 2 |
| `collectEvent` | fn | evt：card/render/panel/stream/泛化 evt 分流 | serve/router.go:174 | 2 |
| `dispatchRender` / `dispatchPanel` / `validatePanelOp` | fn | 呈现意图中继与 PanelOp 合法性（组件前缀） | serve/router.go:215/226/245 | — |
| `handleHostPlugins` / `handleSetPluginEnabled` / `handlePluginSwitch` / `handleEnsurePlugins` | fn | host.* 方法实现 | serve/router.go:287/297/335/344 | 2 |
| `EnsurePlugins` / `EnsurePluginsResult` | fn/struct | 按名幂等挂载 + dependsOn 闭包 | serve/router.go:378/369 | 2 |
| `RegistrySnapshot` / `Registry` | struct/fn | provides/faces/degraded/disabled 快照 | serve/registry.go:14/28 | — |
| `registerProvides` | fn | 能力属主登记（tools 多属主例外） | serve/registry.go:59 | 2 |
| `reconcileConsumes` | fn | 降级不动点算法 | serve/registry.go:87 | 2 |
| `Event` / `Subscriber` / `publish` / `Subscribe` / `Panels` | type/fn | 事件总线与扇出 | serve/events.go:5/11/33/16/55 | 2,3 |
| `SwitchPath` / `LoadPluginSwitchFile` / `FilterMountedFound` | fn | .plugin-switch.json 存取与启动过滤 | serve/plugin_switch.go:23/32/136 | 1 |
| `SetPluginEnabled` / `unmountPlugin` | fn | 禁用即卸载 + fail pending | serve/plugin_switch.go:154/194 | 2 |
| `hostPluginsSnapshot` | fn | host.plugins 载荷 | serve/plugin_switch.go:256 | — |
| `PresentationCap` 等常量 / `PresentationCard` | const/alias | 呈现契约名与 Card 别名 | serve/medium.go:13-42 | — |
| `CallHost` | fn | Medium 直达宿主 L0 方法 | serve/host_call.go:12 | 2,3 |
| `SetDebug` / `debugf` / `debugFrame` | fn | Frame 调试日志（stderr） | serve/debug.go:18/22/35 | — |
| `newJob` / `assign` / `killTree` | fn | Windows Job Object（job_other 为空实现） | serve/job_windows.go / job_other.go | — |

## web（Web Medium）

| 符号 | Kind | 职责摘要 | 位置 | 图 |
|---|---|---|---|---|
| `Options` / `Server` | struct | 配置与 HTTP 服务器状态（hub/replay/uiDirs） | web/server.go:28/51 | 3 |
| `CommandPlane` | iface | slash 命令面接口（app 实现） | web/server.go:45 | 3 |
| `New` | fn | 建 mux、订阅 serve 事件、种子 panel 重放 | web/server.go:79 | 3 |
| `broadcast` | fn | 环形重放（500）+ 慢消费者策略 | web/server.go:133 | 3 |
| `handleEvents` | fn | SSE（replay=1 回放，15s keepalive） | web/server.go:195 | 3 |
| `handleCommand` | fn | POST /api/command → commandPlane | web/server.go:250 | 3 |
| `handleUIAction` | fn | POST /api/ui-action → CallByFace(ui) | web/server.go:270 | 3 |
| `handleCall` | fn | POST /api/call：host/face/plugin 三分支 + turn 状态广播 | web/server.go:308 | 3,4,5 |
| `fillDefaultWorkspace` | fn | session.create 补默认 workspace（ADR-0020） | web/server.go:390 | — |
| `handlePlugins` | fn | 插件目录 + 关系图（nodes/edges/scheme/summary） | web/server.go:502 | — |
| `handlePluginUI` | fn | /plugin-ui/ 静态服务（防路径逃逸） | web/server.go:874 | 3 |
| `shellHTML` / `sdkJS` / `appFS` | var | go:embed 静态资源 | web/embed.go:11/14/20 | 3 |
| `State*` / `graphNode` / `graphEdge` | const/struct | 插件图节点边模型 | web/server.go:427-454 | — |

## 契约层（protocol / plugin / pluginsdk / layout）

| 符号 | Kind | 职责摘要 | 位置 | 图 |
|---|---|---|---|---|
| `Frame` / `FrameError` | struct | 线消息（v/id/type/to/cap/method/payload/error） | protocol/frame.go:16/31 | 1,2 |
| `WriteFrame` / `ReadFrame` | fn | uint32 大端长度 + JSON；16MB 上限 | protocol/frame.go:64/84 | 2,5 |
| `Version` | const | 线协议 v5（To 寻址） | protocol/frame.go:58 | — |
| `Manifest` | struct | plugin.json 全字段（provides/consumes/dependsOn/hostFaces/ui…） | plugin/manifest.go:97 | 1 |
| `UISpec` / `UIMount` / `UIPage` / `UISlot` | struct | Web 面板与页面贡献声明 | plugin/manifest.go:23/54/37/45 | — |
| `LoadManifest` / `Validate` | fn | 读取与校验（BOM 容错、UI-only 约束、组件前缀） | plugin/manifest.go:151/169 | — |
| `CurrentProtocol` | const | manifest 协议上限 = 6 | plugin/manifest.go:123 | — |
| `ValidComponentTag` / `ValidUISlot` / `ReservedCommandNames` | fn/var | 组件标签/槽位/保留命令名校验 | plugin/manifest.go:247/67/138 | — |
| `ResolveEntry` / `EntryExists` / `UIEntryExists` / `UIAssetsExist` | fn | 入口与 UI 资源存在性 | plugin/manifest.go:357/362/325/338 | — |
| `pluginsdk.Server` | struct | 插件内运行时（handlers/pending/stdio） | pluginsdk/server.go:32 | 1 |
| `Handle` / `Serve` / `dispatch` | fn | 注册/读循环/分发（未知方法→method_not_found） | pluginsdk/server.go:52/132/172 | — |
| `Call` / `CallTo` / `callFrame` | fn | 能力外呼 / 点名外呼 | pluginsdk/server.go:87/93/100 | 4 |
| `Emit` / `EmitTo` | fn | evt 广播 / 带 id evt | pluginsdk/server.go:59/72 | — |
| `PanelOp` / `Card` / `RenderIntent` / `StreamPayload` / `SummaryPair` | struct | 呈现线契约 | pluginsdk/presentation.go:25/44/75/137/69 | — |
| `EmitPanel` / `EmitCard` / `EmitRender` / `EmitStream(To)` / `EmitStatus` | fn | 呈现发射器 | pluginsdk/presentation.go:35/51/91/145/168 | 5 |
| `SessionCap`…`ProbeMethod` 常量组 | const | 能力名共享定义（不硬编码目录名） | pluginsdk/presentation.go:117-131 | — |
| `Message` / `ToolCall` / `TurnResult` | struct | 模型上下文共享类型 | pluginsdk/types.go:10/18/25 | — |
| `Doc` / `Page` / `Slot` / `Merged` | struct | 布局模型（main 页五区：top/bottom/left/center/right） | layout/layout.go:38/30/22/44 | — |
| `Load` / `Merge` | fn | 读基座 + 插件增量合并（只增不删） | layout/layout.go:52/119 | 1 |

## 装配层（discovery / assembly）

| 符号 | Kind | 职责摘要 | 位置 | 图 |
|---|---|---|---|---|
| `Found` / `Result` / `Scan` | type/fn | 目录扫描（只观察不启动） | discovery/discovery.go:14/20/36 | 1 |
| `Plan` / `Config` / `Rejected` | struct | 挂载计划（Mounted/Unmounted/Missing/Rejected） | assembly/assembly.go:42/14/50 | 1 |
| `Resolve` | fn | 旧白名单式装配（-assembly 已弃用，仍被 Load 路径保留） | assembly/assembly.go:71 | — |
| `ResolveClosure` | fn | dependsOn 闭包展开（环/未知名/保留名冲突处理） | assembly/autostart.go:15 | 2 |
| `ResolveAutostart` | fn | autostart 根 + UI-only 全挂 → 计划 | assembly/autostart.go:66 | 1 |
| `EffectiveMount` / `ResolveUIMounts` / `ManifestUIContributions` | type/fn | UI mount 裁决（disable/override/winner/order） | assembly/ui.go:13/24/120 | 1 |

## 插件（plugins/，全部进程式）

### agent（provides: loop, agent-presets；dependsOn: session, llm-openai, context-manager）

| 符号 | Kind | 职责摘要 | 位置 |
|---|---|---|---|
| `main` | fn | 注册 loop.turn/loop.cancel/agent-presets.get/config.*/commands.call | plugins/agent/main.go:908 |
| `runTurn` | fn | Agent Loop 主循环（Turn/Step、工具编排、subagent） | plugins/agent/main.go:989 |
| `handleTodo` / `subagentToolName` | fn/const | todo 工具与 run_subagent 实现 | plugins/agent/main.go:698/29 |
| `callTool` | fn | policy → 记账 → tools.call 分发 | plugins/agent/main.go:775 |
| `policyDecide` / `policyDecisionOnError` / `confirmTool` | fn | 策略调用与 fail-closed 决策表、ask 兜底 | plugins/agent/main.go:547/579/619 |
| `appendOne` / `deriveMessages` / `nextTurnNumber` / `sessionWorkspace` / `sessionPermissionMode` | fn | session 能力消费包装 | plugins/agent/main.go:306/319/471/498/512 |
| `prepareContext` / `maybeAutoCompact` / `assembleSystemPrompt` / `llmInfoContextWindow` | fn | context/system-prompt/llm 消费 | plugins/agent/main.go:398/420/347/386 |
| `toolsPluginNames` / `liveProviderFor` / `defaultPluginFor` / `defaultToolsPlugins` | fn/var | host.plugins 快照解析 + 工厂默认表 | plugins/agent/main.go:195/236/178/193 |
| `collectToolSchemas` / `rememberTools` / `toolsOwnerFor` | fn | tools.list 扇出与 tool→owner 映射 | plugins/agent/main.go:363/292/286 |
| `agent` | struct | SDK server、cancel 表、toolOwners、per-session turn 锁 | plugins/agent/main.go:871 |
| `activeScheme` / `ensureSchemePlugins` / `filterTools` / `schemeMaxSteps` | fn | Agent Scheme（config 驱动，经 host.ensurePlugins 拉依赖） | plugins/agent/config.go:111/124/144/164 |
| `loadConfig` / `handleConfigCap` / `handleConfigCommand` | fn | config face（get/set/schema/reload） | plugins/agent/config.go:68/178/251 |
| UI | js | agent-mode-panel(right)、agent-status(bottom) | plugins/agent/ui/main.js |

### session（provides: session, choice）

| 符号 | Kind | 职责摘要 | 位置 |
|---|---|---|---|
| `main` | fn | 注册 session.*、config.*、commands.call、choice.ask/respond | plugins/session/main.go:569 |
| `Fact` / `Message` / `SessionMeta` | struct | JSONL 事实与派生消息、会话元数据（workspace/permissionMode/parent…） | plugins/session/main.go:98/110/123 |
| `registry` / `store` | struct | 会话注册表与单会话文件存储（openStore/append/query/derive） | plugins/session/main.go:188/170 |
| `create` | handler | 建会话（subagent 继承 permissionMode，不抢 Current） | plugins/session/main.go:622 |
| `setPermissionMode` | handler | ADR-0033 权限模式 + session_meta 事实 | plugins/session/main.go:672 |
| `append` / `query` / `derive` | handler | 事实写入 / 读取 / 投影（stubOldToolResults 截断旧工具结果） | plugins/session/main.go:716/732/805 |
| `list` / `info` / `current` / `select` | handler | 会话观测与 Current 切换 | plugins/session/main.go:749/753/781/788 |
| `choice.ask` / `choice.respond` | handler | 审批问询（evt 扇出 + 首答胜出 + 超时） | plugins/session/main.go:937/987 |
| `NormalizePermissionMode` / `EffectivePermissionMode` | fn | ask/workspace_write/full_access 归一 | plugins/session/main.go:150/166 |
| UI | js | session-rail(left)、session-workspace(center)、session-status(bottom)；审批卡与 trace 视图 | plugins/session/ui/main.js |

### llm-openai（provides: llm）

| 符号 | Kind | 职责摘要 | 位置 |
|---|---|---|---|
| `llm.complete` | handler | OpenAI 兼容流式补全（SSE delta → EmitStreamTo） | plugins/llm-openai/main.go:675 |
| `llm.info` / `llm.stats` | handler | contextWindow 与用量统计 | plugins/llm-openai/main.go:705/715 |
| `appendReasoningFact` | fn | reasoning 事实 → session.append | plugins/llm-openai/main.go:473 |
| （noteUsage 推送） | call | usage → context-manager context.noteUsage | plugins/llm-openai/main.go:263 |
| `config.*` / `commands.call` | handler | key/model/baseURL 配置 | plugins/llm-openai/main.go:749/761 |
| UI | js | llm-openai-status(bottom) | plugins/llm-openai/ui/main.js |

### context-manager（provides: system-prompt, context）

| 符号 | Kind | 职责摘要 | 位置 |
|---|---|---|---|
| `Segment` / `store` | struct | 提示词片段库（segments.json 基座 + 运行时注册） | plugins/context-manager/main.go:30/65 |
| `system-prompt.registerSegment/registerContext/assemble` | handler | 组装系统提示词 | plugins/context-manager/main.go:317/328/339 |
| `context.prepare` / `compact` / `noteUsage` / `usage` / `listContext` / `registerSkill` | handler | 上下文准备/摘要/用量/列表 | plugins/context-manager/main.go:358/461/424/479/513/347 |
| `buildCompactHint` / `softBudgetRatio` | fn/const | 0.80 预算比 → suggestCompact | plugins/context-manager/main.go:168/166 |
| `lastLLMUsageFromLog` | fn | 回看 session.query 的 llm_usage 事实 | plugins/context-manager/main.go:260 |
| （tools.list 扇出） | call | 估算工具 token 时列全工具 | plugins/context-manager/main.go:302 |
| UI | js | context-manager-status(bottom) | plugins/context-manager/ui/main.js |

### 工具与策略插件

| 插件 | 关键符号（handler） | 位置 |
|---|---|---|
| filetools | `tools.list` / `tools.call` / `workspace.resolve` | plugins/filetools/main.go:176/180/200 |
| shelltools | `tools.list` / `tools.call` | plugins/shelltools/main.go:184/187 |
| webtools | `tools.list` / `tools.call` / `config.*` | plugins/webtools/main.go:563/567/590 |
| skill-manager | `tools.list` / `tools.call` / `skills.list/get/expand/refreshCatalog`；注册片段 → context-manager | plugins/skill-manager/main.go:129/132/157-198/219 |
| sandbox | `policy.decide` / `policy.rules` / `config.*`；`askSessionChoice` → session choice.ask | plugins/sandbox/main.go:418/463/487/281 |
| project-context | `project-context.load`（AGENTS.md/CLAUDE.md） | plugins/project-context/main.go:40 |

## 浏览器侧（web/static + sdk + 插件 ui）

| 符号/模块 | Kind | 职责摘要 | 位置 |
|---|---|---|---|
| `LiteAgent` | js global | call/callCap/emitUIAction/on/emit/complete 桥（POST /api/call） | sdk/lite-agent.js:1 |
| `main.js` | js entry | Shell boot 顺序（loader/events/panels/settings） | web/static/app/main.js:1 |
| `events.js` | js module | SSE 桥：topic 分发（panel→loader，evt 透传） | web/static/app/events.js:7 |
| `loader.js` | js module | Panel Components 挂载（静态 mounts + 运行时 PanelOp） | web/static/app/loader.js:1 |
| `state.js` / `settings.js` / `plugins-panel.js` | js module | 当前会话状态 / 设置面板 / 插件图面板 | web/static/app/ |
| `shell.html` | html | Shell 骨架（五区槽位宿主） | web/static/shell.html |
| session ui | js | rail/workspace/view/trace/审批卡（最大业务 UI） | plugins/session/ui/main.js |
| `gen_sdk` | fn | sdk/lite-agent.js → web/static/sdk.js 拷贝（go generate） | web/gen_sdk.go:14 |
