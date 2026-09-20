# my-go-lite-agent 依赖图 — DEPENDENCY

> 范围：**除 `plugins/`**。层级：**L1 模块默认**；另附核心 L2 调用链。  
> 边 kind：`imports`（Go import）· `calls`（运行时调用）· `owns`（组合/生命周期）· `implements`（接口实现）· `unverified`。  
> 节点格式：`名称 (kind) ::path:line`。完整符号见 [INDEX.md](INDEX.md)；讲解见 [OVERVIEW.md](OVERVIEW.md)。

---

## 图 A — L0 子系统总览

```mermaid
flowchart TB
  Entries["入口 cmd/* (subsystem) ::cmd/liteagent-cli/main.go:7"]
  Medium["Render Medium internal/app + web (subsystem) ::internal/app/app.go:5"]
  Host["Host serve (subsystem) ::serve/serve.go:23"]
  Contracts["契约 protocol/pluginsdk/plugin (subsystem) ::protocol/frame.go:16"]
  Mount["发现与装配 discovery/assembly (subsystem) ::discovery/discovery.go:36"]
  UI["Web Shell + SDK (subsystem) ::web/static/app/main.js:1"]
  Plugins["进程外插件 plugins/* (out-of-scope) ::(not analyzed)"]

  Entries -->|imports| Medium
  Medium -->|imports| Host
  Medium -->|imports| Mount
  Medium -->|imports| UI
  Host -->|imports| Mount
  Host -->|imports| Contracts
  Mount -->|imports| Contracts
  Medium -->|owns| Host
  Host -->|owns| Plugins
  UI -->|calls| Medium
  Plugins -->|implements| Contracts
```

---

## 图 B — L1 模块 imports（Go 包级）

证据来自各源文件 import 块（已抽样打开）。

```mermaid
flowchart LR
  subgraph entries [Entries]
    CLI["cmd/liteagent-cli (pkg) ::main.go:7"]
    SRV["cmd/liteagent-server (pkg) ::main.go:8"]
  end

  subgraph medium [Medium]
    APP["internal/app (pkg) ::app.go:5"]
    WEB["web (pkg) ::server.go:2"]
    MDANSI["render/mdansi (pkg) ::mdansi.go:30"]
  end

  subgraph host [Host]
    SERVE["serve (pkg) ::serve.go:2"]
  end

  subgraph mount [Mount]
    DISC["discovery (pkg) ::discovery.go:2"]
    ASM["assembly (pkg) ::assembly.go:2"]
    LAYOUT["layout (pkg) ::layout.go:2"]
  end

  subgraph contracts [Contracts]
    PROTO["protocol (pkg) ::frame.go:2"]
    SDK["pluginsdk (pkg) ::server.go:8"]
    PLUG["plugin (pkg) ::manifest.go:2"]
  end

  CLI -->|imports| APP
  SRV -->|imports| APP
  APP -->|imports| SERVE
  APP -->|imports| DISC
  APP -->|imports| ASM
  APP -->|imports| WEB
  APP -->|imports| LAYOUT
  APP -->|imports| PROTO
  APP -->|imports| SDK
  APP -->|imports| MDANSI
  APP -->|imports| PLUG
  WEB -->|imports| SERVE
  WEB -->|imports| ASM
  WEB -->|imports| DISC
  WEB -->|imports| PLUG
  SERVE -->|imports| PROTO
  SERVE -->|imports| SDK
  SERVE -->|imports| DISC
  SERVE -->|imports| PLUG
  SERVE -->|imports| ASM
  SDK -->|imports| PROTO
  DISC -->|imports| PLUG
  ASM -->|imports| DISC
  ASM -->|imports| PLUG
  MDANSI -.->|external goldmark| EXT["goldmark (third-party) ::go.mod:6"]
```

**核实过的 import 证据（抽样）**：

| from | to | 证据 |
|------|----|------|
| `cmd/liteagent-cli` | `internal/app` | `cmd/liteagent-cli/main.go:5` |
| `cmd/liteagent-server` | `internal/app` | `cmd/liteagent-server/main.go:6` |
| `internal/app` | `serve`,`discovery`,`assembly`,`protocol` | `internal/app/app.go:14-17` |
| `internal/app` | `web`,`layout`,`assembly` | `internal/app/web.go:11-13` |
| `internal/app` | `pluginsdk`,`render/mdansi`,`serve` | `internal/app/paint.go:8-10` |
| `web` | `serve`,`assembly`,`discovery`,`plugin` | `web/server.go:18-21` |
| `serve` | `protocol`,`discovery` | `serve/serve.go:12-13` |
| `serve` | `assembly`,`plugin`,`pluginsdk`,`protocol` | `serve/router.go:10-14` |
| `pluginsdk` | `protocol` | `pluginsdk/server.go:17` |
| `discovery` | `plugin` | `discovery/discovery.go:10` |
| `assembly` | `discovery` / `plugin` | `assembly/autostart.go:8` / `ui.go:9` |

---

## 图 C — 核心运行时调用链（L2/L3，REPL 金路径）

```mermaid
flowchart TB
  MAIN["main (fn) ::cmd/liteagent-cli/main.go:7"] -->|calls| CLI["CLI (fn) ::internal/app/cli.go:24"]
  CLI -->|calls| RUNREPL["runREPL (fn) ::internal/app/repl.go:14"]
  RUNREPL -->|calls| RESASM["resolveAssembly (fn) ::internal/app/app.go:69"]
  RESASM -->|calls| SCAN["Scan (fn) ::discovery/discovery.go:36"]
  RESASM -->|calls| RAUTO["ResolveAutostart (fn) ::assembly/autostart.go:66"]
  RAUTO -->|calls| RCLOSE["ResolveClosure (fn) ::assembly/autostart.go:15"]
  RUNREPL -->|calls| STARTM["startMounted (fn) ::internal/app/app.go:105"]
  STARTM -->|calls| START["Start (fn) ::serve/serve.go:109"]
  START -->|calls| LAUNCH["launch (method) ::serve/transport.go:69"]
  LAUNCH -->|owns| PROC["proc (struct) ::serve/transport.go:42"]
  START -->|calls| REGP["registerProvides (method) ::serve/registry.go:59"]
  START -->|calls| RECON["reconcileConsumes (method) ::serve/registry.go:87"]
  RUNREPL -->|calls| LOOP["runREPLLoop (fn) ::internal/app/repl.go:51"]
  LOOP -->|calls| TURN["runLoopTurn (fn) ::internal/app/callx.go:19"]
  TURN -->|calls| CALLP["callPlugin (fn) ::internal/app/callx.go:12"]
  CALLP -->|calls| CBP["CallByPlugin (method) ::serve/serve.go:209"]
  CBP -->|calls| CBY["callByPlugin (method) ::serve/transport.go:264"]
  CBY -->|calls| CALL["call (method) ::serve/transport.go:253"]
  CALL -->|calls| CSTREAM["callStream (method) ::serve/transport.go:286"]
  CSTREAM -->|calls| CONCE["callOnce (method) ::serve/transport.go:372"]
  CONCE -->|calls| WRITE["writeTo (method) ::serve/transport.go:235"]
  WRITE -->|calls| WF["WriteFrame (fn) ::protocol/frame.go:58"]
  LOOP -->|calls| CP["commandPlane (struct) ::internal/app/commands.go:17"]
  CP -->|calls| CBF["CallByFace (fn) ::serve/serve.go:170"]
  LOOP -->|calls| WIRE["wireRenderer (fn) ::internal/app/paint.go:145"]
  WIRE -->|calls| SUB["Subscribe (method) ::serve/events.go:20"]
```

---

## 图 D — Host 入站 Frame 路由（L2）

```mermaid
flowchart TB
  RL["readLoop (method) ::serve/transport.go:124"] -->|calls| RF["ReadFrame (fn) ::protocol/frame.go:78"]
  RL -->|calls| HFP["handleFromPlugin (method) ::serve/transport.go:224"]
  HFP -->|req| RR["routeRequest (method) ::serve/router.go:17"]
  HFP -->|res| COMP["complete (method) ::serve/router.go:147"]
  HFP -->|evt| CE["collectEvent (method) ::serve/router.go:178"]

  RR -->|host.to/cap| HOSTSW{"host method switch"}
  HOSTSW -->|ensurePlugins| EP["EnsurePlugins (method) ::serve/router.go:372"]
  HOSTSW -->|plugins| HP["handleHostPlugins (method) ::serve/router.go:281"]
  HOSTSW -->|setPluginEnabled| SPE["SetPluginEnabled (method) ::serve/plugin_switch.go:154"]
  HOSTSW -->|pluginSwitch| HPS["handlePluginSwitch (method) ::serve/router.go:329"]

  RR -->|deferred agent.*| HAF["handleAgentFromPlugin (method) ::serve/router.go:423"]
  HAF -->|request| AR["AgentRequest (method) ::serve/context.go:75"]
  HAF -->|inject| AI["AgentInject (method) ::serve/context.go:91"]
  HAF -->|confirm| AA["askApproval (method) ::serve/events.go:63"]
  AR -->|calls| CBD["agentDerive (method) ::serve/context.go:25"]

  RR -->|empty to| REJ["to_required error"]
  RR -->|else| FW["forwardTo (method) ::serve/router.go:80"]
  FW -->|calls| WRITE2["writeTo (method) ::serve/transport.go:235"]
  EP -->|calls| RCLOSE2["ResolveClosure (fn) ::assembly/autostart.go:15"]

  CE -->|presentation.card| CARD["recordCard (method) ::serve/medium.go:53"]
  CE -->|presentation.render| DR["dispatchRender (method) ::serve/router.go:209"]
  CE -->|presentation.panel| DP["dispatchPanel (method) ::serve/router.go:220"]
  DP -->|validates| VPO["validatePanelOp (method) ::serve/router.go:239"]
  CE -->|publish| PUB["publish (method) ::serve/events.go:82"]
```

**说明**：`CBD` 仅用于 deferred `agent.request`；`callByCapOwner`（`serve/context.go:13`）在这条特例路径上按 `provides` 查属主。主路由 **不** 按 capability 分支。

---

## 图 E — Web Medium（L2）

```mermaid
flowchart TB
  SMAIN["main (fn) ::cmd/liteagent-server/main.go:8"] -->|calls| SERVF["Server (fn) ::internal/app/server.go:13"]
  SERVF -->|calls| WEBRUN["runWebAndOptionalREPL (fn) ::internal/app/web.go:26"]
  WEBRUN -->|calls| LLOAD["Load (fn) ::layout/layout.go:52"]
  WEBRUN -->|calls| LMERGE["Merge (fn) ::layout/layout.go:119"]
  WEBRUN -->|calls| UIM["ResolveUIMounts (fn) ::assembly/ui.go:24"]
  WEBRUN -->|calls| STARTM2["startMounted (fn) ::internal/app/app.go:105"]
  WEBRUN -->|calls| NEW["New (fn) ::web/server.go:83"]
  NEW -->|owns| WSRV["Server (struct) ::web/server.go:51"]
  NEW -->|calls| SUB2["Subscribe (method) ::serve/events.go:20"]
  NEW -->|calls| RA["RegisterApproval (fn) ::serve/events.go:40"]
  NEW -->|route| HC["handleCall (method) ::web/server.go:379"]
  NEW -->|route| HE["handleEvents (method) ::web/server.go:204"]
  NEW -->|route| HPL["handlePlugins (method) ::web/server.go:573"]
  HC -->|to=host| CH["CallHost (fn) ::serve/host_call.go:12"]
  HC -->|hostFace| CBF2["CallByFace (fn) ::serve/serve.go:170"]
  HC -->|else| CBP2["CallByPlugin (method) ::serve/serve.go:209"]
  SHELL["main.js (Shell) ::web/static/app/main.js:1"] -->|fetch| API["/api/call|/api/layout|/plugin-ui"]
  LSDK["lite-agent.js (Platform Module) ::sdk/lite-agent.js:1"] -->|fetch| API
  API --> HC
```

---

## 边抽查清单（≥5，均已打开源文件）

| # | 边 | 证据 | 结论 |
|---|----|------|------|
| 1 | `internal/app` calls `serve.Start` | `app.go:109` `serve.Start(mounted)` | ok |
| 2 | `resolveAssembly` calls `discovery.Scan` + `assembly.ResolveAutostart` | `app.go:73,77` | ok |
| 3 | `runLoopTurn` → `CallByPlugin("agent","loop","turn")` | `callx.go:24` via `callx.go:12-14` | ok（L0 点名） |
| 4 | Host `routeRequest` 对空 `to` 返回 `to_required` | `router.go:58-66` | ok（不按 cap 路由） |
| 5 | `EnsurePlugins` 调用 `assembly.ResolveClosure` | `router.go:376` | ok |
| 6 | `web.handleCall` 三分支 host / face / plugin | `server.go:435-441` | ok |
| 7 | `pluginsdk` → `protocol` | `server.go:17` | ok |
| 8 | Shell 仅映射五区 | `main.js:19-25` + `plugin.UISlots` `manifest.go:64` | ok（ADR-0031） |

---

## 不入图 / unverified

| 项 | 处理 |
|----|------|
| `plugins/**` 内部符号 | 范围排除，仅图 A 以 out-of-scope 节点示意 |
| `internal/app` 各测试文件 | 不入运行时图 |
| `assembly.Resolve` 白名单路径 | 仍存在（`assembly.go:71`），**无日常调用方**；`-assembly` 在 `resolveAssembly` 被忽略，故不画入金路径 |
| JS 模块细粒度 call 图 | 仅画 Shell → HTTP 面；loader/events/plugins-panel 内部未展开 |
| 第三方 goldmark 实现细节 | 外部依赖，仅 dotted 边 |
