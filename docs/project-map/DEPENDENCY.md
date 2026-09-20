# Project Map — DEPENDENCY（server + plugins）

> 节点格式：`名称 (kind) ::path:line`；边标注类型。符号全表见 [INDEX.md](INDEX.md)。
> 所有边均由 import 块或调用点逐一核实；未经核实的边标 `unverified`。
> 范围：liteagent-server 宿主侧 + plugins/（10 个插件）；CLI Medium（liteagent-cli）已删除。

## 图例

| 边 | 含义 |
|---|---|
| imports | Go 包 import |
| spawns | fork 插件子进程（stdio Frame） |
| calls | 函数调用（含经 Frame 的运行时调用，注明 method） |
| publishes | evt 扇出（SSE / 事件总线） |
| serves | HTTP 路由到处理函数 |

---

## 图 1 — L0：Go 包依赖（imports）

```mermaid
flowchart LR
  MAIN["main (bin) ::cmd/liteagent-server/main.go:8"] -->|imports| APP["app (pkg) ::internal/app/app.go:5"]
  APP -->|imports| SERVE["serve (pkg) ::serve/serve.go:2"]
  APP -->|imports| WEB["web (pkg) ::web/server.go:2"]
  APP -->|imports| ASM["assembly (pkg) ::assembly/assembly.go:2"]
  APP -->|imports| DISC["discovery (pkg) ::discovery/discovery.go:2"]
  APP -->|imports| LAY["layout (pkg) ::layout/layout.go:3"]
  APP -->|imports| PLG["plugin (pkg) ::plugin/manifest.go:2"]
  APP -->|imports| SDK["pluginsdk (pkg) ::pluginsdk/server.go:2"]
  APP -->|imports| PROTO["protocol (pkg) ::protocol/frame.go:2"]
  WEB -->|imports| SERVE
  WEB -->|imports| ASM
  WEB -->|imports| DISC
  WEB -->|imports| PLG
  SERVE -->|imports| ASM
  SERVE -->|imports| DISC
  SERVE -->|imports| PLG
  SERVE -->|imports| SDK
  SERVE -->|imports| PROTO
  ASM -->|imports| DISC
  ASM -->|imports| PLG
  DISC -->|imports| PLG
  SDK -->|imports| PROTO
  PG["plugins/* (10 binaries) ::plugins/*/main.go"] -->|imports| SDK
  PG -->|imports| PROTO
```

要点：`protocol`、`plugin` 是零依赖叶子（契约层）；宿主（serve/web）与插件（pluginsdk）只共享
契约层，业务包互不 import —— 进程边界在编译期就成立。

---

## 图 2 — L1：serve 包内部（宿主内核）

```mermaid
flowchart TB
  START["serve.Start (fn) ::serve/serve.go:105"]
  REG["serve.registerProvides (fn) ::serve/registry.go:59"]
  RECON["serve.reconcileConsumes (fn) ::serve/registry.go:87"]
  LAUNCH["serve.launch (fn) ::serve/transport.go:69"]
  RLOOP["serve.readLoop (fn) ::serve/transport.go:124"]
  HANDLE["serve.handleFromPlugin (fn) ::serve/transport.go:224"]
  ROUTE["serve.routeRequest (fn) ::serve/router.go:20"]
  FWD["serve.forwardTo (fn) ::serve/router.go:76"]
  COMPLETE["serve.complete (fn) ::serve/router.go:143"]
  COLLECT["serve.collectEvent (fn) ::serve/router.go:174"]
  ENSURE["serve.EnsurePlugins (fn) ::serve/router.go:378"]
  CLOSURE["assembly.ResolveClosure (fn) ::assembly/autostart.go:15"]
  CALLONCE["serve.callOnce (fn) ::serve/transport.go:372"]
  CBP["serve.CallByPlugin (fn) ::serve/serve.go:205"]
  CBF["serve.CallByFace (fn) ::serve/serve.go:166"]
  CALLHOST["serve.CallHost (fn) ::serve/host_call.go:12"]
  ALIVE["serve.ensureAlive (fn) ::serve/transport.go:185"]
  UNHEALTHY["serve.markUnhealthy (fn) ::serve/transport.go:137"]
  PUB["serve.publish (fn) ::serve/events.go:33"]
  SUB["serve.Subscribe (fn) ::serve/events.go:16"]
  SWITCH["serve.SetPluginEnabled (fn) ::serve/plugin_switch.go:154"]
  UNMOUNT["serve.unmountPlugin (fn) ::serve/plugin_switch.go:194"]
  PROC["proc (struct) ::serve/transport.go:42"]
  WAIT["wait (struct) ::serve/transport.go:23"]

  START -->|calls| REG
  START -->|calls| LAUNCH
  START -->|calls| RECON
  LAUNCH -->|owns/spawns| PROC
  LAUNCH -->|calls| RLOOP
  RLOOP -->|calls| HANDLE
  HANDLE -->|req| ROUTE
  HANDLE -->|res| COMPLETE
  HANDLE -->|evt| COLLECT
  ROUTE -->|host.*| ENSURE
  ROUTE -->|to 点名| FWD
  ENSURE -->|calls| CLOSURE
  ENSURE -->|calls| LAUNCH
  FWD -->|owns| WAIT
  FWD -->|calls| ALIVE
  COMPLETE -->|resolves| WAIT
  COLLECT -->|publishes| PUB
  CBP -->|calls| CALLONCE
  CBF -->|calls| CALLONCE
  CALLONCE -->|owns| WAIT
  CALLONCE -->|calls| ALIVE
  ALIVE -->|calls| LAUNCH
  UNHEALTHY -->|calls| RECON
  SWITCH -->|calls| UNMOUNT
  UNMOUNT -->|calls| RECON
  PUB -->|fans out| SUB
```

---

## 图 3 — L1：Web Medium 与浏览器 Shell

```mermaid
flowchart LR
  subgraph Go["web 包 ::web/server.go"]
    NEW["web.New (fn) ::web/server.go:79"]
    HC["handleCall (fn) ::web/server.go:308"]
    HCMD["handleCommand (fn) ::web/server.go:250"]
    HUI["handleUIAction (fn) ::web/server.go:270"]
    HPLG["handlePlugins (fn) ::web/server.go:502"]
    HEV["handleEvents (fn) ::web/server.go:195"]
    HLY["handleLayout (fn) ::web/server.go:418"]
    HPU["handlePluginUI (fn) ::web/server.go:874"]
    BR["broadcast (fn) ::web/server.go:133"]
    CP["commandPlane (struct) ::internal/app/commands.go:17"]
  end
  SDK["LiteAgent (js) ::sdk/lite-agent.js"]
  MAINJS["main.js (js entry) ::web/static/app/main.js"]
  EVJS["events.js (js) ::web/static/app/events.js:7"]
  LDJS["loader.js (js) ::web/static/app/loader.js:1"]
  PPJS["plugins-panel.js (js) ::web/static/app/plugins-panel.js"]
  SUI["session ui (js) ::plugins/session/ui/main.js:1"]

  NEW -->|serves| HEV
  NEW -->|serves| HC
  NEW -->|serves| HCMD
  NEW -->|serves| HUI
  NEW -->|serves| HPLG
  NEW -->|serves| HLY
  NEW -->|serves| HPU
  HCMD -->|calls| CP
  BR -->|SSE| EVJS
  MAINJS -->|imports| EVJS
  MAINJS -->|imports| LDJS
  MAINJS -->|imports| PPJS
  SDK -->|POST /api/call| HC
  SUI -->|uses| SDK
  EVJS -->|panel evt| LDJS
  SUI -->|mounted by| LDJS
```

---

## 图 4 — L1/L2：运行时插件调用图（Frame 边，经 serve 转发）

节点为插件进程；边为 `CallTo(plugin, cap, method)`，每条边标注了代表性调用点。
宿主（serve）与 Web（app）以特殊节点并入。

```mermaid
flowchart TB
  HOST["host (serve.Server) ::serve/serve.go:23"]
  WEBAPP["web/app (medium) ::internal/app/web.go:26"]
  AGENT["agent ::plugins/agent/main.go:908"]
  SESSION["session ::plugins/session/main.go:569"]
  LLM["llm-openai ::plugins/llm-openai/main.go:675"]
  CM["context-manager ::plugins/context-manager/main.go:293"]
  FT["filetools ::plugins/filetools/main.go"]
  ST["shelltools ::plugins/shelltools/main.go"]
  WT["webtools ::plugins/webtools/main.go"]
  SKM["skill-manager ::plugins/skill-manager/main.go"]
  SBX["sandbox ::plugins/sandbox/main.go"]
  PC["project-context ::plugins/project-context/main.go:40"]
  TOOLS["tools 提供方（FT/ST/WT/SKM）"]

  WEBAPP -->|"loop.turn / loop.cancel, agent-presets.get"| AGENT
  WEBAPP -->|"session.*"| SESSION
  WEBAPP -->|"host.* (CallHost)"| HOST
  AGENT -->|"session.append/derive/query/create/info :315/:324/:429/:1416"| SESSION
  AGENT -->|"session.choice.ask（ask 兜底）:632"| SESSION
  AGENT -->|"llm.complete/info :1227/:387"| LLM
  AGENT -->|"system-prompt.assemble/registerSegment; context.prepare/compact/noteUsage :348/:406/:443"| CM
  AGENT -->|"policy.decide :559"| SBX
  AGENT -->|"skills.expand/refreshCatalog :644/:695"| SKM
  AGENT -->|"project-context.load :666"| PC
  AGENT -->|"tools.list/tools.call :367/:814"| TOOLS
  AGENT -->|"host.plugins; host.ensurePlugins(agent/config.go:128)"| HOST
  LLM -->|"context.noteUsage :263"| CM
  LLM -->|"session.append(reasoning) :481"| SESSION
  CM -->|"session.query :262"| SESSION
  CM -->|"tools.list :302"| TOOLS
  SBX -->|"choice.ask :287"| SESSION
  SKM -->|"context.registerSkill :219"| CM
  SESSION -.->|"evt: choice.ask 审批卡（SSE 扇出）"| SUI
  SUI -.->|"choice.respond（ui main.js:972）"| SESSION
```

注：`tools` 能力是多属主（filetools/shelltools/webtools/skill-manager 都 provides tools），
由 agent 在本地记录 tool→owner 映射后点名调用（agent:286-304）；Host 不合并 tools。

---

## 图 5 — L3：一条 Agent Turn（时序）

```mermaid
sequenceDiagram
  participant B as 浏览器 session-view
  participant W as web.handleCall ::web/server.go:308
  participant S as serve.callOnce ::serve/transport.go:372
  participant A as agent.runTurn ::plugins/agent/main.go:989
  participant SE as session ::plugins/session/main.go:805
  participant CM as context-manager ::plugins/context-manager/main.go:358
  participant L as llm-openai ::plugins/llm-openai/main.go:675
  participant P as sandbox ::plugins/sandbox/main.go:418

  B->>W: POST /api/call {to:agent, cap:loop, method:turn}
  W->>B: SSE status=running (:358)
  W->>S: CallByPlugin(agent)
  S->>A: Frame req loop.turn
  A->>SE: session.derive (:324)
  A->>CM: context.prepare (:406)
  A->>L: llm.complete (:1227)
  L-->>S: evt stream delta (EmitStreamTo)
  S->>W: collectEvent → publish
  W-->>B: SSE topic=stream
  L-->>A: res（content+tool_calls）
  A->>P: policy.decide (:559)
  P->>SE: choice.ask (:287→session:937)
  SE-->>B: evt choice.ask（审批卡）
  B->>SE: choice.respond（ui main.js:972）
  SE-->>P: res {value}
  P-->>A: res {action}
  A->>SE: tools 事实 append（:306 appendOne）
  A-->>S: res turnResult
  S-->>W: CallResult
  W->>B: SSE status=idle (:377) + HTTP res
```
