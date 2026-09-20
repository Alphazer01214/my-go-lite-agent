# Project Map — OVERVIEW（server + plugins 全量分析）

> 范围：`cmd/liteagent-server` 入口、宿主运行时（`serve` / `web` / `internal/app`）、
> 基础设施包（`protocol` / `plugin` / `pluginsdk` / `discovery` / `assembly` / `layout`），
> 以及 `plugins/` 下全部 10 个插件（含各插件 ui/ 前端入口的角色定位）。
> CLI Medium（原 `cmd/liteagent-cli` 与 `internal/app` 的 repl/cli/readline/paint/vt/legacy_compat）
> 已整体删除：宿主只剩 Web Medium。
> 依据：**当前工作区代码**（存在未提交改动：`serve/context.go`、`serve/session.go` 已删除）。
> 本文只描述代码中真实存在的关系；与文档冲突处以代码为准（见文末"冲突与未核实项"）。
> 配套：[DEPENDENCY.md](DEPENDENCY.md)（图）· [INDEX.md](INDEX.md)（符号索引）·
> [RUNTIME.md](RUNTIME.md)（启动到 agent 运行的全过程）。

## 一句话

这是一个 **进程插件式 Agent 宿主**：一个 Go 进程（liteagent-server）按 `plugin.json` 清单把多个独立
可执行文件（插件）拉起为子进程，用 **stdin/stdout 上的长度前缀 JSON Frame**（`protocol` 包）通信，
按 **插件名点对点转发** 请求（Frame.To，不做能力星型路由）；浏览器 Web Shell 通过 HTTP + SSE 与宿主通信，
业务能力（会话日志、Agent Loop、LLM、上下文、工具、审批）全部由插件实现，宿主只做
发现、装配、路由、生命周期与呈现事件中继。

## 分层（L0 → L2）

### L0 子系统（Go 包）

| 子系统 | 目录 | 职责 |
|---|---|---|
| 入口 | `cmd/liteagent-server` | 8 行薄壳，调 `app.Server()`（main.go:8） |
| App 装配 | `internal/app` | flag 解析、装配计划、命令面、Web 启动（`server.go` `web.go` `app.go` `commands.go` `callx.go`） |
| 插件宿主 | `serve` | 拉起/监控/重启插件进程，Frame 路由，注册表与降级，事件扇出，插件开关 |
| Web Medium | `web` | HTTP + SSE 服务器，内嵌 Shell 静态资源，插件 UI 静态服务，插件图 API |
| 发现 | `discovery` | 扫描 `<pluginsDir>/<child>/plugin.json`，只观察不启动（discovery.go:36） |
| 装配 | `assembly` | Autostart 根 + dependsOn 闭包 → 挂载计划；UI mounts 裁决 |
| 布局 | `layout` | layout.json 基座 + 插件增量页/槽合并（layout.go:119） |
| Manifest | `plugin` | plugin.json 结构与校验（plugin/manifest.go） |
| 协议 | `protocol` | Frame 线格式：uint32 大端长度 + JSON（protocol/frame.go:64/84） |
| 插件 SDK | `pluginsdk` | 插件侧运行时（Serve/Handle/CallTo/Emit*）+ 呈现线契约 |
| 插件 | `plugins/*`（10 个） | 业务能力的独立进程 |
| 浏览器 SDK | `sdk/lite-agent.js`（经 web/gen_sdk.go 拷贝内嵌为 web/static/sdk.js） | Shell 内 Panel Components 的 `LiteAgent.*` 桥 → POST /api/call |

依赖方向（imports，全部由各文件 import 块核实）：`app → {serve, web, assembly, discovery, layout, plugin, pluginsdk, protocol}`；
`web → {serve, assembly, discovery, plugin}`；`serve → {assembly, discovery, plugin, pluginsdk, protocol}`；
`discovery/assembly → plugin`；`pluginsdk → protocol`；`plugins/* → {pluginsdk, protocol}`（部分只用 pluginsdk）。
`protocol` 与 `plugin` 是零内部依赖叶子。图见 DEPENDENCY.md 图 1。

### L1 模块（宿主内核 serve 与 Web Medium）

- **宿主核 serve.Server** `serve/serve.go:23`：持有 `plugins map[name]*proc`、`provides`（能力→插件，
  仅观测/降级用，路由不查它）、`catalog`、`pending`（等待中的调用）、`subs`（事件订阅者）、
  `disabled`（插件开关）。启动入口 `Start` `serve/serve.go:105`（软失败：单个插件挂不上不影响整体）。
- **Frame 路由器** `serve/router.go`：`routeRequest:20`（host.* 方法分派 / To 点名转发）、
  `forwardTo:76`（改写 fwd-id、登记 pending、超时定时器）、`complete:143`（回包还原原 id）、
  `collectEvent:174`（evt 中继：card/render/panel/stream/泛化 evt）、
  `EnsurePlugins:378`（按名挂载 + dependsOn 闭包）、`EnsurePluginsResult:369`。
- **进程传输** `serve/transport.go`：`proc:42`、`launch:69`（exec + Windows Job Object）、
  `readLoop:124`、`markUnhealthy:137`（死亡 → 失败 pending 调用 + 重算注册表）、
  `ensureAlive:185`（按需重启，callStreamOn 第二次尝试）、`callOnce:372`（Host 侧调用原语）、
  `Close:320`（stdin EOF → 宽限 2s → killTree）。
- **注册表与降级** `serve/registry.go`：`registerProvides:59`（非 tools 能力唯一属主，冲突仅告警）、
  `reconcileConsumes:87`（迭代到不动点：consumes 无主 → 降级并撤回其 provides，传递扩散）。
- **事件扇出** `serve/events.go`：`Event:5`（topic：presentation|status|stream|panel|evt）、
  `Subscribe:16`、`publish:33`（panel 环形缓存 256 供重放）。
- **插件开关** `serve/plugin_switch.go`：`.plugin-switch.json` 持久化；`SetPluginEnabled:154`
  禁用即卸载并 fail pending 调用（`unmountPlugin:194`）、`FilterMountedFound:136`（启动过滤）。
- **呈现契约别名** `serve/medium.go`（cap/method 常量、`Cards:45`）、`serve/render.go`
  （RenderIntent/PanelOp 等 = pluginsdk 类型别名，一份定义）。
- **Host L0 面** `serve/host_call.go:12` `CallHost`：plugins / ensurePlugins / setPluginEnabled / pluginSwitch。
- **Web Medium** `web/server.go`：路由 `/`、`/events`(SSE, `handleEvents:195`)、`/api/command`、
  `/api/ui-action`、`/api/call`（`handleCall:308`）、`/api/plugins`（插件图, `handlePlugins:502`）、
  `/api/layout`、`/plugin-ui/<name>/`（路径逃逸防护, `handlePluginUI:874`）、`/app/`；
  `New:79` 订阅 serve 事件并桥接到 SSE `broadcast:133`（环形重放 500 + 慢消费者丢弃策略）。
- **命令面** `internal/app/commands.go`：`commandPlane:17`，Web Shell 的原生 slash 路由；`/api/command`
  → `handleOut:92`（原生 /help /lp /refresh + 插件命令 → `serve.CallByFace(commands)`）；
  `refresh:37` 对声明 config face 的插件广播 config.reload。
- **插件侧运行时** `pluginsdk/server.go`：`Server:32`、`Handle:52`（注册 `cap.method`）、
  `Serve:132`（读循环）、`dispatch:172`（未知方法回 method_not_found）、
  `Call:87`/`CallTo:93`（点名外呼）、`Emit/EmitTo:59/72`。

### L2 类型（核心）

`serve.Server` / `serve.proc`（serve/transport.go:42）/ `serve.wait`（:23）/
`serve.RegistrySnapshot`（serve/registry.go:14）/ `web.Server` / `web.Options`（web/server.go:28）/
`plugin.Manifest`（plugin/manifest.go:97）/ `plugin.UISpec`（:23）/
`protocol.Frame`（protocol/frame.go:16）/ `pluginsdk.Server` / `pluginsdk.PanelOp` /
`pluginsdk.RenderIntent`（pluginsdk/presentation.go:75）/ `assembly.Plan`（assembly/assembly.go:42）/
`assembly.EffectiveMount`（assembly/ui.go:13）/ `layout.Merged`（layout/layout.go:44）/
session 的 `Fact`（plugins/session/main.go:98）与 `registry`/`store`（:188/:170）。

## 主流程（入口符号链）

### 1. 启动链

```
main (cmd/liteagent-server/main.go:8)
 → app.Server (internal/app/server.go:12)          # 必需 -plugins 与 -serve
 → runWeb (internal/app/web.go:26)
    → resolveAssembly (internal/app/app.go:29)     # discovery.Scan + assembly.ResolveAutostart
    → layout.Load → layout.Merge                    # 基座 layout.json + ManifestUIContributions(assembly/ui.go:120)
    → assembly.ResolveUIMounts (assembly/ui.go:24)
    → startMounted (internal/app/app.go:51)        # LoadPluginSwitchFile → FilterMountedFound
       → serve.Start (serve/serve.go:105)          # 逐插件 launch，软失败
       → srv.SetCatalog / SetPluginsDir / SetDisabledSet
    → callx.callPlugin("session","create")         # web.go:71 建 default 会话（cwd 为 workspace）
    → probeCommandFaces (app.go:86)                # 声明 commands 却无 handler 的告警
    → newCommandPlane (internal/app/commands.go:24)
    → web.New (web/server.go:79) → Serve(ln)       # HTTP+SSE 上线；订阅 serve 事件扇出
```

### 2. Frame 转发链（插件→插件）

```
插件子进程 stdout → serve.readLoop (transport.go:124)
 → handleFromPlugin (transport.go:224)
    ├ req → routeRequest (router.go:20)
    │        ├ to=host → handleEnsurePlugins / handleHostPlugins / handleSetPluginEnabled / handlePluginSwitch
    │        └ else  → forwardTo (router.go:76)   # 改写 fwd-id、登记 pending、超时定时器
    ├ res → complete (router.go:143)              # 按 id 找 pending，还原原 id 回写 caller
    └ evt → collectEvent (router.go:174)          # presentation.* / 泛化 evt → publish
```

### 3. Web 调用链（浏览器→插件）

```
浏览器 LiteAgent.call (sdk/lite-agent.js) → POST /api/call
 → web.handleCall (web/server.go:308)
    ├ to=host            → serve.CallHost (serve/host_call.go:12)
    ├ cap∈{config,commands,ui} → serve.CallByFace (serve/serve.go:166)  # 校验 hostFaces 声明，杜绝绕过
    └ else               → srv.CallByPlugin (serve/serve.go:205)
        → callStreamOn → callOnce (transport.go:372) → writeTo → 插件
```

### 4. 一条 Agent Turn（核心业务链，L3）

```
session-view (plugins/session/ui/main.js) → LiteAgent.callCap('agent','loop','turn')
 → /api/call → handleCall：先广播 status=running（web/server.go:355-358）
 → agent.runTurn (plugins/agent/main.go:989)
    → per-session TryLock（:995-999，busy 即拒）
    → session.derive（重建 Model Context, :324 → session/main.go:805）
    → scheme.ensureSchemePlugins → host.ensurePlugins（plugins/agent/config.go:128）
    → context-manager context.prepare（软探针 + suggestCompact→context.compact→session.append, :398/:1169）
    → llm-openai llm.complete（流式 delta 经 EmitStreamTo → collectEvent → publish → SSE, llm:675）
    → 有 tool_calls：policy.decide（sandbox, :559；ask → sandbox askSessionChoice:287
       → session choice.ask:937 → evt 扇出 → session UI 审批卡 → choice.respond:987 回填）
    → tools.call 到属主插件（owner 由 tools.list 记录, :810-826）
    → 每步事实 append 进 session（appendOne, :306）
    → EmitRender（markdown/message/summary 意图 → SSE）
 → handleCall 收尾广播 status=idle（web/server.go:372-378）
```

工具提供方（filetools/shelltools/webtools/skill-manager）只实现 `tools.list/tools.call`；
read-only 工具并行、写串行（agent:1317-1374）；severity 从 tools.list 透传给 policy；
策略对"已挂载但 decide 出错"fail-closed（agent:547-584）。

## 插件目录速览（10 个，均为独立进程 + 可选 ui/main.js）

| 插件 | provides | 运行时被谁调用 | 说明 |
|---|---|---|---|
| `agent` | loop, agent-presets | Web（loop.turn / loop.cancel）、web.handlePlugins（agent-presets.get） | Agent Loop 编排者；唯一声明 dependsOn 的插件 |
| `session` | session, choice | agent、llm-openai、context-manager、sandbox、skill-manager、Web、宿主启动 | JSONL Session Log + 审批/选择问询（choice.ask/respond） |
| `llm-openai` | llm | agent | OpenAI 兼容流式 provider；主动推 usage 与 reasoning 事实 |
| `context-manager` | system-prompt, context | agent、llm-openai、skill-manager | 系统提示词组装 + prepare/compact/usage |
| `filetools` | tools, workspace | agent、context-manager | 工作区读写（含 workspace.resolve） |
| `shelltools` | tools | agent、context-manager | 跨平台 shell |
| `webtools` | tools | agent、context-manager | web_fetch / web_search |
| `skill-manager` | tools, skills | agent、context-manager | 技能发现 / `$` 触发展开 / 注册片段到 context-manager |
| `sandbox` | policy | agent | 审批策略（fail-closed，内部走 choice.ask） |
| `project-context` | project-context | agent | 读 AGENTS.md/CLAUDE.md |

只有 `agent/session/context-manager/llm-openai` 标 `autostart`；工具与 sandbox 由 agent 的
dependsOn 闭包或 scheme.ensurePlugins 拉起（`assembly.ResolveClosure:15`）。

插件 UI 组件（经 /plugin-ui/ + loader.js 挂到五区槽位）：session-rail(left)、
session-workspace(center)、session-status/agent-status/llm-openai-status/context-manager-status(bottom)、
agent-mode-panel(right)。

## 如何自己读

1. 先读 `protocol/frame.go`（100 行，一切通信的底座）→ `pluginsdk/server.go`（插件看到的世界）。
2. 再读 `serve/serve.go` + `serve/transport.go` + `serve/router.go`（宿主看到的世界）。
3. 启动装配：`internal/app/web.go` 一条线到底。
4. 业务主线：`plugins/agent/main.go` 的 `runTurn`（:989 起）对照 `plugins/session/main.go` 的 derive/append。
5. 前端：`web/static/app/main.js`（boot）→ `events.js`（SSE）→ `loader.js`（组件挂载）→
   `plugins/session/ui/main.js`（最大的业务 UI，含审批卡与 trace 视图）。

## 冲突与未核实项

- **陈旧注释（文档 vs 代码）**：`plugins/session/ui/main.js:25-26` 注释称 Turn 状态/取消走
  `/api/session`、`/api/turn/cancel` —— `web/server.go` 并无这两个路由；实际取消走
  `LiteAgent.callCap('agent','loop','cancel')`（main.js:1247）→ `/api/call`。
- **未提交删除**：工作区已删 `serve/context.go`、`serve/session.go`（git 状态 D）；本文按删除后的代码描述。
- **硬编码 vs 注释**：`web/server.go:581` 读取 Agent Scheme 时硬编码
  `CallByPlugin("agent","agent-presets",…)`；同文件 ：778 附近注释声称 agent 顶点"从不硬编码插件名"
  （该注释仅指图顶点由注册表解析，读取 scheme 的调用点确实硬编码了 "agent"）。
- **manifest consumes 不完整**：`plugins/agent/plugin.json` 的 consumes 只声明
  system-prompt/context/session/llm，但代码还经 `defaultPluginFor`（agent/main.go:178）调用
  policy(sandbox)/skills(skill-manager)/project-context —— 这些运行时依赖不参与
  `reconcileConsumes` 的降级计算。
- **未核实（只读静态分析）**：浏览器端运行时行为（SSE 重放、custom element upgrade 时序）
  仅依据代码推断，未启动浏览器验证；`serve/job_windows.go` 的 Job Object 行为未做运行验证。
- 边抽查：启动链、/api/call 三分支、agent→session/llm/policy 各调用点、llm→context.noteUsage、
  sandbox→choice.ask 等均已逐一打开 file:line 核实命中。
