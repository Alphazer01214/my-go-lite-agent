# my-go-lite-agent 项目地图 — OVERVIEW

> 范围：**除 `plugins/` 以外**的全部 Host / Medium / 契约 / 工具链代码。  
> 深度：L1 模块默认 + 核心路径 L2/L3。  
> 证据优先级：**代码第一，文档为辅**。规范名词见 [CONTEXT.md](../../CONTEXT.md)；架构决策见 [docs/adr/](../adr/)。  
> 配套：[DEPENDENCY.md](DEPENDENCY.md) · [INDEX.md](INDEX.md)

---

## 1. 这是什么

轻量 **Go Agent 运行时**：**Host 薄内核 + 进程外插件**。核心包零第三方（解析库除外：`goldmark`、`x/term`、Windows 下 `x/sys`）。

```text
用户输入
   │
   ▼
┌──────────── Host（L0 薄内核，serve/）────────┐
│  Discovery · Assembly · 进程生命周期          │
│  按插件名转发（Frame.to + 不透明 payload）   │
│  通用事件/应答 · hostFaces · ensurePlugins   │
└───┬──────────┬──────────┬──────────┬────────┘
    │          │          │          │
 session     llm     context-manager tools
（进程外插件，本分析不展开）
    ▲          ▲          ▲
    └──────────┴────┬─────┴──────────┘
                    │
                 agent（插件，provides loop）
```

**Host 不实现 Agent Loop。** 会话事实、Turn/Step 编排、工具调度都在插件里；Host 只做 L0 寻址与生命周期（ADR-0030）。

---

## 2. 怎么分层（L0 / L1）

| 层 | 含义 | 本仓库对应 |
|----|------|------------|
| **L0** | 按**插件名**转发、进程生命周期、通用挂载、通用事件/应答、hostFaces、`host.ensurePlugins` | `serve/`；Medium 只经 Host 使用 |
| **L1** | 能力名/方法、领域 payload 字段 | **不得**出现在 Host 路由分支；领域语义在插件与 `pluginsdk` 契约 |
| **L2** | 插件目录名字面量做调度硬编码 | Host/Medium 生产源不应出现；**调用方点名目标插件是 L0 寻址**（如 REPL 点名 `"agent"` 发 `loop.turn`） |

**Deferred 例外已清零（ADR-0034）**：Host 不再特例处理任何 `agent.*`——`agent.confirm` 由 session 插件通用 choice 取代（sandbox 点名 `session.choice.ask`），`agent.request`/`inject` 由 agent 插件直连 `session.derive`/`session.append`。

**文档 vs 代码**：CLAUDE.md 速查表写 ADR-0030 的 `CurrentProtocol=5`（0031 升到 6）。代码里 `plugin.CurrentProtocol = 6`（`plugin/manifest.go:123`）。**以代码为准**。Frame 线协议版本是独立的 `protocol.Version = 5`（`protocol/frame.go:52`）。

---

## 3. 仓库布局（非 plugins）

| 子系统 | 路径 | 一句话 |
|--------|------|--------|
| 入口 | `cmd/liteagent-cli` / `cmd/liteagent-server` | 两扇 thin main，全部进 `internal/app` |
| Medium 运行时 | `internal/app/` | flag 分发、装配启动、REPL、slash 命令面、Web 启动、CLI 绘制 |
| Host | `serve/` | L0 内核：进程、路由、事件、ensure、plugin switch |
| 契约 | `protocol/` `pluginsdk/` `plugin/` | Frame 线格式 · 插件作者 SDK/共享类型 · Manifest |
| 发现与装配 | `discovery/` `assembly/` | 扫描磁盘 · Autostart+dependsOn 闭包 + UI 裁决 |
| Web Medium | `web/` + `web/static/` + `sdk/` | HTTP 面、SSE、Shell 五区、Platform Module |
| Layout | `layout/` + `layout.json` | 页面/槽位几何真源（五区 `top\|bottom\|left\|center\|right`） |
| 绘制 | `render/mdansi/` | Markdown → ANSI（CLI Medium） |
| 构建 | `scripts/` | 跨平台构建 + 出厂插件清单 |

Go module：`github.com/tomori/my-go-lite-agent`（`go.mod:1`），Go 1.25.6。

---

## 4. 启动数据流（装配 → Host）

代码链（与 `docs/modules/README.md` 启动图一致）：

```text
discovery.Scan(pluginsDir)                    // discovery.go:36
  → assembly.ResolveAutostart(res)            // autostart.go:66  日常真源
  → serve.Start(plan.Mounted) + SetCatalog    // serve.go:109 / app.go:105-116
  → serve.SetPluginsDir + SetDisabledSet      // ADR-0032 开关
  → [Web] layout.Load + ManifestUIContributions + layout.Merge
       + assembly.ResolveUIMounts             // web.go:35-58
  → Medium 注册 Subscribe / CommandPlane
```

要点：

- **Autostart + dependsOn** 是日常挂载真源（ADR-0021）。`-assembly` 仍被 flag 接受，但 **`resolveAssembly` 直接忽略其文件**（`internal/app/app.go:69-71`）。
- **软失败**（ADR-0017/0022）：单插件 launch 失败只 stderr 警告，Host 不退出（`serve.go:143-146`）。
- **运行中懒挂载**：Agent Scheme `dependsPlugins` → Frame `host.ensurePlugins` → `assembly.ResolveClosure`（`serve/router.go:372-419`）。
- UI-only 插件（`entry == ""`）进 `mountedUI`，不起进程（`serve.go:137-141`；`autostart.go:72-74` 也把 UI-only 收进根集）。

---

## 5. 主流程：REPL 金路径（L2/L3）

```text
cmd/liteagent-cli/main.go:7  main
  → internal/app/cli.go:24   CLI()
    → repl.go:14             runREPL(pluginsDir, …)
      → app.go:69            resolveAssembly → Scan + ResolveAutostart
      → app.go:105           startMounted → serve.Start + SetCatalog/…
      → repl.go:35           newCommandPlane
      → repl.go:47           runREPLLoop
        → repl.go:96         runLoopTurn(srv, "", line)
          → callx.go:19      组 payload {input, allowSubagent}
          → callx.go:24      callPlugin(srv, "agent", "loop", "turn", …)
            → callx.go:12    srv.CallByPlugin
              → serve.go:209 CallByPlugin
              → transport.go:264 callByPlugin → call → callStream → callOnce
              → transport.go:389 writeTo(plugin stdin)  // protocol.WriteFrame
```

插件侧（进程外，本图只到边界）：`pluginsdk.Server.Serve`（`pluginsdk/server.go:132`）读 stdin Frame → `dispatch` by `cap.method` → stdout 回 `res`/`evt`。

Host 回程：

```text
transport.go:124  readLoop
  → transport.go:224 handleFromPlugin
      req → router.go:17  routeRequest
            host 方法 | 空 to 拒绝 | forwardTo(to)
      res → router.go:147 complete  (waitHost→channel / waitPlugin 恢复 origID)
      evt → router.go:178 collectEvent
            presentation.render/panel/card/stream → publish
  → events.go:82  publish → Subscriber (CLI turnRenderer / Web SSE)
```

CLI 绘制：`paint.go:145 wireRenderer` 订阅 `stream|status|presentation`，用 `pluginsdk.RenderIntent` + `render/mdansi` 画终端。

---

## 6. 主流程：Web Medium

```text
cmd/liteagent-server/main.go:8  main
  → internal/app/server.go:13   Server()
    → web.go:26                 runWebAndOptionalREPL
      → web.go:35               layout.Load("layout.json")
      → web.go:40-51            ManifestUIContributions → layout.Merge
      → web.go:55               ResolveUIMounts
      → web.go:60               startMounted
      → web.go:87               web.New(Options{Srv, Plan, Layout, …})
      → web.go:97               hs.Serve(ln)
```

`web/server.go:83 New` 注册的 L0 HTTP 面：

| 路由 | Handler | 作用 |
|------|---------|------|
| `/events` | handleEvents | SSE + replay ring |
| `/api/call` | handleCall | 按插件名点名 / host / hostFaces |
| `/api/command` | handleCommand | slash → CommandPlane |
| `/api/ui-action` | handleUIAction | UI Action → 目标插件 `ui.action` |
| `/api/plugins` | handlePlugins | Plugin Graph（Discovery 全量） |
| `/api/layout` | handleLayout | 合并后 Layout |
| `/plugin-ui/` | handlePluginUI | 插件 UI Entry 静态资源 |
| `/app/` | embed `static/app` | Shell 模块 |
| `/sdk/lite-agent.js` | handleSDK | Platform Module |

`handleCall` 分支（`web/server.go:435-441`）——**代码事实**：

1. `to == host` → `serve.CallHost`（L0 方法：plugins / ensurePlugins / setPluginEnabled / pluginSwitch）
2. `cap ∈ {config,commands,ui}` → `serve.CallByFace`（须 Manifest 声明 hostFace）
3. 否则 → `srv.CallByPlugin(to, cap, method, payload)`（L0 点名）

Shell 前端（`web/static/app/main.js:19-26`）只把 `top|bottom|left|center|right` 映射到 DOM region；领域 UI 是插件 Panel Component。

---

## 7. Host 路由细节（关键符号）

| 符号 | 位置 | 角色 |
|------|------|------|
| `Frame` | `protocol/frame.go:16` | `v,id,type,to,cap,method,payload,error`；Host 只用 `to` 寻址 |
| `Version` | `protocol/frame.go:52` | Frame 线版本 = 5（≠ Manifest protocol） |
| `Server` | `serve/serve.go:23` | 进程表、pending、catalog、degraded、approvals、switch |
| `Start` | `serve/serve.go:109` | 启动挂载集，软失败 |
| `CallByPlugin` | `serve/serve.go:209` | Host/Medium 点名插件调用 |
| `CallByFace` | `serve/serve.go:170` | hostFaces：config\|commands\|ui，未声明则拒 |
| `routeRequest` | `serve/router.go:17` | host 方法 / forwardTo |
| `forwardTo` | `serve/router.go:80` | id 改写 `fwd-N` + 超时 |
| `EnsurePlugins` | `serve/router.go:372` | 幂等懒挂载 + dependsOn 闭包 |
| `registerProvides` / `reconcileConsumes` | `serve/registry.go:59,87` | **观测/降级**，不是路由表（ADR-0030） |
| `CallHost` | `serve/host_call.go:12` | Medium 命中的 Host L0 方法 |
| `publish` / `Subscribe` | `serve/events.go:82,20` | 多消费者事件总线 |
| 通用 evt 中继 | `serve/router.go` `collectEvent` | 无 id 且非 presentation 的插件 evt → Topic `evt`，原样透传（ADR-0034） |
| `CurrentProtocol` | `plugin/manifest.go:123` | Manifest/UI 契约上限 = **6** |
| `ResolveAutostart` / `ResolveClosure` | `assembly/autostart.go:66,15` | 日常挂载真源 / 懒挂载共用闭包算法 |
| `Scan` | `discovery/discovery.go:36` | 只看见、不启动 |

`provides` 注册：非 `tools` 能力冲突会报错但仍继续（软失败）；`tools` 允许多提供方，Host **不合并、不路由**（`registry.go:64-79`）。

---

## 8. Medium / slash / 绘制

| 符号 | 位置 | 角色 |
|------|------|------|
| `CLI` / `Server` | `internal/app/cli.go:24` / `server.go:13` | 两入口 |
| `commandPlane` | `internal/app/commands.go:17` | 原生 `/help /lp /refresh /exit`；其余按插件名 → `CallByFace(..., "commands", "call")` |
| `runLoopTurn` | `internal/app/callx.go:19` | Medium 点名 `agent` 发 `loop.turn`（L0 寻址） |
| `turnRenderer` / `wireRenderer` | `internal/app/paint.go:15,145` | CLI 终端绘制 |
| `runLegacyDomain` | `internal/app/legacy_compat.go:81` | **仅** `L0_TEST_COMPAT=1` 测试兼容；产品 argv 无领域 flag |
| `mdansi.Render` | `render/mdansi/mdansi.go:30` | Markdown→ANSI |
| `pluginsdk.Server` | `pluginsdk/server.go:32` | 插件作者 Handle/Call/Emit/Serve |
| `pluginsdk.CallTo` | `pluginsdk/server.go:93` | 插件间按名调用（经 Host 转发） |

Slash 原生命令集：`plugin.ReservedCommandNames` = help/lp/refresh/exit（`plugin/manifest.go:138-143`）。同名插件在 Assembly/Discovery 路径会被拒绝（`ConflictsWithNativeCommand`）。

---

## 9. 关键符号表（模块级）

完整索引见 [INDEX.md](INDEX.md)。这里只列「读代码必碰」的入口级符号：

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `main` (cli) | fn | `cmd/liteagent-cli/main.go:7` | CLI 入口 |
| `main` (server) | fn | `cmd/liteagent-server/main.go:8` | Web 入口 |
| `CLI` | fn | `internal/app/cli.go:24` | L0 flag 分发 |
| `runREPL` | fn | `internal/app/repl.go:14` | 挂载 + 交互环 |
| `runWebAndOptionalREPL` | fn | `internal/app/web.go:26` | Web 装配启动 |
| `startMounted` | fn | `internal/app/app.go:105` | Host 启动胶水 |
| `resolveAssembly` | fn | `internal/app/app.go:69` | Scan + Autostart |
| `serve.Start` | fn | `serve/serve.go:109` | Host 生命周期起点 |
| `serve.Server` | struct | `serve/serve.go:23` | Host 状态 |
| `routeRequest` | method | `serve/router.go:17` | Frame 入站路由 |
| `protocol.Frame` | struct | `protocol/frame.go:16` | 线消息 |
| `plugin.Manifest` | struct | `plugin/manifest.go:97` | plugin.json |
| `discovery.Scan` | fn | `discovery/discovery.go:36` | 目录发现 |
| `assembly.Plan` | struct | `assembly/assembly.go:42` | 挂载计划 |
| `assembly.ResolveAutostart` | fn | `assembly/autostart.go:66` | 日常真源 |
| `web.Server` / `web.New` | struct/fn | `web/server.go:51,83` | Web Medium |
| `layout.Doc` / `layout.Merge` | struct/fn | `layout/layout.go:38,119` | 槽位几何 |
| `pluginsdk.Server` | struct | `pluginsdk/server.go:32` | 插件运行时 |

---

## 10. 如何自己读

1. **先定入口**：CLI 走 `internal/app/cli.go`；Web 走 `internal/app/server.go`。  
2. **跟 Host**：`serve.Start` → `transport.launch` → `readLoop` → `router.routeRequest`。  
3. **跟一次对话**：`repl.runREPLLoop` → `callx.runLoopTurn` → `CallByPlugin("agent","loop","turn")`。领域逻辑不在本仓 Host 代码里。  
4. **跟 Web**：`web.New` 路由表 + `web/static/app/main.js` 五区装载 + `sdk/lite-agent.js`。  
5. **对契约**：`protocol/frame.go`（线）· `plugin/manifest.go`（元数据）· `pluginsdk/presentation.go`（Presentation/Stream/Panel）。  
6. **对决策**：ADR-0030（L0-only）· 0021（Autostart）· 0027（hostFaces）· 0031（五区）· 0032（插件开关）。

图节点与本表 **同源**；抽查依赖边时直接打开 [INDEX.md](INDEX.md) 的 `path:line`。

---

## 11. 未核实 / 边界

| 项 | 状态 |
|----|------|
| `plugins/**` 业务实现 | **未分析**（用户范围排除）；图中不画插件内部符号 |
| `web/static` JS 运行时行为 | 静态源码已读（main/loader/events/sdk）；未在浏览器实测 |
| `sdk/` 与 `web/static/sdk.js` 是否字节一致 | 测试 `web/sdk_sync_test.go:11 TestSDKCopiesMatch` 存在；**本回合未跑测试** |
| `docs/modules/*` 与代码 | 整体一致；CLAUDE.md 中 `CurrentProtocol=5` 一句与代码 `=6` 以代码为准（见 §2） |
| `assembly.Resolve`（白名单） | 代码仍在（`assembly/assembly.go:71`），**日常路径不用**；`-assembly` 被 `resolveAssembly` 忽略 |
| `internal/app` 中出现 `"agent"`/`"session"`/`"loop"` 字符串 | **核实为 Medium L0 点名寻址**，不是 Host 路由分支；Host 内领域分支仅 deferred `agent.*` |
| 第三方依赖 | `go.mod` 仅 goldmark + x/term（+ x/sys indirect）；核心零第三方结论成立 |

---

## 12. 口述摘要（≤10 行）

1. 这是 **Host 薄内核 + 进程外插件** 的 Go Agent 运行时；Host 不跑 Agent Loop。  
2. 分层：`cmd/*` → `internal/app`（Medium）→ `serve`（Host L0）→ `protocol/pluginsdk/plugin`（契约）+ `discovery/assembly`（挂载）。  
3. 日常装配真源 = **Autostart + dependsOn 闭包**；`-assembly` 已废弃忽略。  
4. 金路径：REPL 输入 → `CallByPlugin("agent","loop","turn")` → Host 按 **插件名** 转发 Frame → 插件进程处理 → evt/res 回 Medium。  
5. Web Medium 只有 L0 HTTP 面：call/command/ui-action/plugins/layout/SSE。  
6. `provides/consumes` 只做观测与 degraded，**不是路由表**。  
7. Deferred 已清零（ADR-0034）：Host 无任何 agent 域特例；问答走 session 插件 choice。  
8. Manifest/UI 协议上限 `CurrentProtocol=6`；Frame 线版本 `protocol.Version=5`。  
9. `plugins/` 实现不在本图内；抽查请对照 INDEX 的入口符号与 DEPENDENCY 边。
