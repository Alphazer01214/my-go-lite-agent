# my-go-lite-agent 符号索引 — INDEX

> 范围：**除 `plugins/`**。路径相对仓库根 `E:\Projects\my-go-lite-agent\`。  
> 与 [OVERVIEW.md](OVERVIEW.md) / [DEPENDENCY.md](DEPENDENCY.md) 同源；抽查依赖边时先来这里定位。

图例：`kind` = pkg | struct | fn | method | const | type | var | js

---

## 入口

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `main` | fn | `cmd/liteagent-cli/main.go:7` | CLI 入口 → `app.CLI` |
| `main` | fn | `cmd/liteagent-server/main.go:8` | Web 入口 → `app.Server` |

---

## internal/app — Medium 运行时

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `package app` | pkg | `internal/app/app.go:5` | 双入口共享 Medium 运行时 |
| `CLI` | fn | `internal/app/cli.go:24` | L0 flag 分发 |
| `Server` | fn | `internal/app/server.go:13` | Web 入口 flag → `runWebAndOptionalREPL` |
| `runDiscover` | fn | `internal/app/app.go:33` | `-discover` |
| `resolveAssembly` | fn | `internal/app/app.go:69` | Scan + Autostart；忽略 `-assembly` |
| `startMounted` | fn | `internal/app/app.go:105` | `serve.Start` + catalog/switch |
| `probePlugin` / `roundtrip` | fn | `internal/app/app.go:136,152` | 诊断：单 Frame 往返 |
| `runCallPlugin` | fn | `internal/app/app.go:197` | `-call-plugin` 点名诊断 |
| `probeCommandFaces` | fn | `internal/app/app.go:224` | 探测 commands 面 |
| `runREPL` | fn | `internal/app/repl.go:14` | 挂载 + REPL |
| `runREPLLoop` | fn | `internal/app/repl.go:51` | stdin 交互环 |
| `runInvoke` | fn | `internal/app/invoke.go:24` | `-invoke` 点名调用 |
| `runLoopTurn` | fn | `internal/app/callx.go:19` | `agent.loop.turn` 点名 |
| `cancelLoopTurn` | fn | `internal/app/callx.go:41` | `agent.loop.cancel` |
| `callPlugin` | fn | `internal/app/callx.go:12` | L0 点名封装 |
| `commandPlane` | struct | `internal/app/commands.go:17` | slash 路由 |
| `newCommandPlane` | fn | `internal/app/commands.go:27` | 构造 |
| `handle` / `handleOut` | method | `internal/app/commands.go:95,107` | 执行 `/cmd` |
| `refresh` | method | `internal/app/commands.go:40` | `/refresh` + config.reload |
| `turnRenderer` | struct | `internal/app/paint.go:15` | CLI 绘制状态 |
| `wireRenderer` | fn | `internal/app/paint.go:145` | 订阅 Host 信号 |
| `cliToolApproval` | fn | `internal/app/paint.go:190` | CLI 审批面 |
| `runWebAndOptionalREPL` | fn | `internal/app/web.go:26` | Web 装配启动 |
| `webCommandPlane` | struct | `internal/app/web.go:16` | `web.CommandPlane` 实现 |
| `runLegacyDomain` | fn | `internal/app/legacy_compat.go:81` | 测试兼容域 flag（`L0_TEST_COMPAT=1`） |
| `readLineRaw` | fn | `internal/app/readline.go:13` | 终端行编辑 + Tab 补全 |
| `enableVirtualTerminal` | fn | `internal/app/vt_windows.go:15` | Windows VT |

---

## serve — Host L0 内核

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `package serve` | pkg | `serve/serve.go:2` | Host |
| `Server` | struct | `serve/serve.go:23` | 进程/pending/catalog/事件/审批/开关 |
| `DefaultCallTimeout` | const | `serve/serve.go:17` | 30s |
| `DefaultShutdownGrace` | const | `serve/serve.go:20` | 2s |
| `HostCap` | const | `serve/serve.go:56` | `"host"` |
| `Start` | fn | `serve/serve.go:109` | 启动挂载集（软失败） |
| `SetCatalog` | method | `serve/serve.go:59` | Discovery 目录 |
| `SetPluginsDir` | method | `serve/serve.go:67` | 根目录 + switch 加载 |
| `MountedPluginNames` | method | `serve/serve.go:76` | `/lp` |
| `DegradedNames` | method | `serve/serve.go:93` | 降级名单 |
| `CallByFace` | fn | `serve/serve.go:170` | hostFaces 调用 |
| `CallByPlugin` | method | `serve/serve.go:209` | 按插件名点名 |
| `MarshalPayload` | fn | `serve/serve.go:214` | JSON 编码辅助 |
| `proc` | struct | `serve/transport.go:42` | 单插件进程句柄 |
| `wait` | struct | `serve/transport.go:23` | pending 请求 |
| `CallResult` | struct | `serve/transport.go:37` | res + events |
| `launch` | method | `serve/transport.go:69` | 起进程 + Job + readLoop |
| `readLoop` | method | `serve/transport.go:124` | 读 stdout Frame |
| `markUnhealthy` | method | `serve/transport.go:137` | 崩溃 → 失败 pending + reconcile |
| `ensureAlive` | method | `serve/transport.go:185` | 拉起/重启 |
| `handleFromPlugin` | method | `serve/transport.go:224` | req/res/evt 分发 |
| `writeTo` | method | `serve/transport.go:235` | 写插件 stdin |
| `call` / `callByPlugin` | method | `serve/transport.go:253,264` | Host 主调内部原语 |
| `callStream` / `callStreamOn` | method | `serve/transport.go:286,292` | 带 evt 回调的调用 |
| `callOnce` | method | `serve/transport.go:372` | 分配 `host-N` + 超时 |
| `Close` | method | `serve/transport.go:320` | EOF + grace + killTree |
| `routeRequest` | method | `serve/router.go:17` | 入站 req 路由 |
| `forwardTo` | method | `serve/router.go:80` | 按 `to` 转发 + fwd-id |
| `complete` | method | `serve/router.go:147` | res 完结 |
| `collectEvent` | method | `serve/router.go:178` | evt 收集/广播 |
| `dispatchRender` | method | `serve/router.go:209` | presentation.render |
| `dispatchPanel` | method | `serve/router.go:220` | presentation.panel |
| `validatePanelOp` | method | `serve/router.go:239` | Panel 前缀/槽位/props 校验 |
| `handleEnsurePlugins` | method | `serve/router.go:338` | host.ensurePlugins |
| `EnsurePlugins` | method | `serve/router.go:372` | 幂等懒挂载 |
| `EnsurePluginsResult` | struct | `serve/router.go:363` | ensure 响应 |
| `handleAgentFromPlugin` | method | `serve/router.go:423` | deferred agent.* |
| `handleHostPlugins` | method | `serve/router.go:281` | host.plugins 快照 |
| `handleSetPluginEnabled` | method | `serve/router.go:291` | ADR-0032 开关 |
| `RegistrySnapshot` | struct | `serve/registry.go:14` | 观测快照 |
| `Registry` | fn | `serve/registry.go:28` | 导出快照 |
| `registerProvides` | method | `serve/registry.go:59` | 能力属主索引（观测） |
| `reconcileConsumes` | method | `serve/registry.go:87` | degraded 不动点 |
| `callByCapOwner` | method | `serve/context.go:13` | deferred 特例：按 provides 查属主 |
| `agentDerive` / `agentAppend` | method | `serve/context.go:25,48` | session 公开契约调用 |
| `AgentRequest` | method | `serve/context.go:75` | Session Log 不变量 |
| `AgentInject` | method | `serve/context.go:91` | 注入 model-visible 消息 |
| `AgentRequestResult` | struct | `serve/context.go:63` | request 结果 |
| `PresentationCap` 等常量 | const | `serve/medium.go:10-38` | presentation/ui/commands 契约名 |
| `PresentationCard` | type | `serve/medium.go:42` | = `pluginsdk.Card` |
| `Cards` / `recordCard` | method | `serve/medium.go:45,53` | Card 观测 |
| `Event` / `Subscriber` | struct | `serve/events.go:9,15` | 事件总线 |
| `Subscribe` | method | `serve/events.go:20` | 多消费者订阅 |
| `RegisterApproval` | fn | `serve/events.go:40` | 审批面注册 |
| `askApproval` | method | `serve/events.go:63` | 并行询问，先应答胜 |
| `publish` | method | `serve/events.go:82` | 扇出 |
| `Panels` | method | `serve/events.go:104` | panel ring |
| `TruncateRunes` | fn | `serve/render.go:29` | 截断 |
| `SetDebug` | fn | `serve/debug.go:18` | stderr Frame 日志 |
| `SwitchPath` / `LoadPluginSwitchFile` | fn | `serve/plugin_switch.go:23,32` | ADR-0032 持久化 |
| `FilterMountedFound` | fn | `serve/plugin_switch.go:136` | 启动过滤 disabled |
| `SetPluginEnabled` | method | `serve/plugin_switch.go:154` | 启用/禁用 + 卸载 |
| `CallHost` | fn | `serve/host_call.go:12` | Medium → Host L0 方法 |
| `newJob` / `jobHolder` | fn/struct | `serve/job_windows.go:18,14` | Windows Job Object |
| `Message`/`ToolCall`/`TurnResult` | type | `serve/session.go:12-18` | pluginsdk 别名 |

---

## protocol — Frame 线格式

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Frame` | struct | `protocol/frame.go:16` | Host↔Plugin 消息 |
| `FrameError` | struct | `protocol/frame.go:28` | 结构化错误 |
| `TypeReq`/`TypeRes`/`TypeEvt` | const | `protocol/frame.go:41-43` | 消息类型 |
| `Version` | const | `protocol/frame.go:52` | Frame 线版本 = **5** |
| `maxFrameSize` | const | `protocol/frame.go:55` | 16<<20 |
| `WriteFrame` | fn | `protocol/frame.go:58` | uint32 BE + JSON |
| `ReadFrame` | fn | `protocol/frame.go:78` | 解码一帧 |

---

## pluginsdk — 插件作者 API / 共享契约

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Server` | struct | `pluginsdk/server.go:32` | 插件内 Frame 运行时 |
| `Request` / `Handler` | type | `pluginsdk/server.go:21,29` | 入站 req |
| `New` | fn | `pluginsdk/server.go:42` | 绑定 stdio |
| `Handle` / `Serve` | method | `pluginsdk/server.go:52,132` | 注册 / 主循环 |
| `Emit` / `EmitTo` | method | `pluginsdk/server.go:59,72` | evt |
| `Call` / `CallTo` | method | `pluginsdk/server.go:87,93` | 经 Host 出站调用 |
| `PanelOp` | struct | `pluginsdk/presentation.go:25` | set\|clear |
| `Card` | struct | `pluginsdk/presentation.go:44` | Presentation Card |
| `RenderIntent` / `RenderKind` | struct | `pluginsdk/presentation.go:75,60` | 主窗渲染意图 |
| `SummaryPair` | struct | `pluginsdk/presentation.go:69` | summary 行 |
| `StreamPayload` | struct | `pluginsdk/presentation.go:137` | stream 信号 |
| `PresentationCap` 等 | const | `pluginsdk/presentation.go:12-21` | 契约名 |
| `SessionCap`/`AgentCap`/`LoopCap`/… | const | `pluginsdk/presentation.go:117-124` | 能力名契约 |
| `ProbeCap`/`ProbeMethod` | const | `pluginsdk/presentation.go:129-130` | echo 诊断 |
| `Message`/`ToolCall`/`TurnResult` | struct | `pluginsdk/types.go:10,18,25` | 模型可见类型 |

---

## plugin — Manifest

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Manifest` | struct | `plugin/manifest.go:97` | plugin.json |
| `CurrentProtocol` | const | `plugin/manifest.go:123` | **6**（Manifest/UI） |
| `CommandSpec` | struct | `plugin/manifest.go:16` | commands[] |
| `UISpec`/`UIPage`/`UISlot`/`UIMount` | struct | `plugin/manifest.go:23,37,45,54` | UI 面 |
| `UISlots` | var | `plugin/manifest.go:64` | `top\|bottom\|left\|center\|right` |
| `ValidUISlot` / `ValidMountSlot` | fn | `plugin/manifest.go:67,80` | 槽位校验 |
| `ValidHostFace` | fn | `plugin/manifest.go:126` | config\|commands\|ui |
| `ReservedCommandNames` | var | `plugin/manifest.go:138` | help/lp/refresh/exit |
| `ConflictsWithNativeCommand` | method | `plugin/manifest.go:146` | 原生命令冲突 |
| `LoadManifest` | fn | `plugin/manifest.go:151` | 读+校验（剥 BOM） |
| `Validate` | method | `plugin/manifest.go:169` | 必填/协议范围 |
| `ResolveEntry` | method | `plugin/manifest.go:357` | 可执行路径 |
| `ValidComponentTag` | fn | `plugin/manifest.go:247` | 自定义元素标签 |

---

## discovery

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Found` | struct | `discovery/discovery.go:14` | 目录 + Manifest |
| `Result` | struct | `discovery/discovery.go:20` | Plugins + Errors |
| `ScanError` | struct | `discovery/discovery.go:26` | 单目录失败 |
| `Scan` | fn | `discovery/discovery.go:36` | 扫描 `<root>/*/plugin.json` |

---

## assembly

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Config`/`UIConfig`/`UIOverride` | struct | `assembly/assembly.go:14,21,29` | 装配文件（deprecated 日常） |
| `Plan` | struct | `assembly/assembly.go:42` | Mounted/Unmounted/Missing/Rejected |
| `Rejected` | struct | `assembly/assembly.go:51` | 拒绝原因 |
| `Load` | fn | `assembly/assembly.go:57` | 读装配文件 |
| `Resolve` | fn | `assembly/assembly.go:71` | 白名单交集（**非日常**） |
| `ResolveClosure` | fn | `assembly/autostart.go:15` | dependsOn 闭包算法 |
| `ResolveAutostart` | fn | `assembly/autostart.go:66` | **日常挂载真源** |
| `EffectiveMount` | struct | `assembly/ui.go:13` | UI 裁决结果 |
| `ResolveUIMounts` | fn | `assembly/ui.go:24` | Manifest mounts + Assembly.ui |
| `ManifestUIContributions` | fn | `assembly/ui.go:120` | 插件 ui.pages 贡献 |

---

## layout

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Doc`/`Page`/`Slot` | struct | `layout/layout.go:38,30,22` | 磁盘 layout.json |
| `Merged` | struct | `layout/layout.go:44` | 合并结果 |
| `Trust` | type | `layout/layout.go:14` | full\|isolated |
| `Load` | fn | `layout/layout.go:52` | 读+校验 |
| `Validate` | method | `layout/layout.go:68` | 必须含 page `main` |
| `Contribution` | struct | `layout/layout.go:112` | 插件加法贡献 |
| `Merge` | fn | `layout/layout.go:119` | 基座 + 贡献 |

---

## web — Web Medium

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Options` | struct | `web/server.go:28` | Medium 配置 |
| `CommandPlane` | interface | `web/server.go:45` | slash 面 |
| `Server` | struct | `web/server.go:51` | HTTP + SSE hub |
| `Event` | struct | `web/server.go:72` | SSE 载荷 |
| `New` | fn | `web/server.go:83` | 路由注册 + 订阅 Host |
| `Serve`/`Close` | method | `web/server.go:133,138` | HTTP 生命周期 |
| `handleIndex` | method | `web/server.go:172` | Shell HTML |
| `handleEvents` | method | `web/server.go:204` | SSE + replay |
| `handleCommand` | method | `web/server.go:259` | slash |
| `handleToolApproval` | method | `web/server.go:314` | 审批应答 |
| `handleUIAction` | method | `web/server.go:341` | UI Action |
| `handleCall` | method | `web/server.go:379` | L0 点名 / host / face |
| `fillDefaultWorkspace` | fn | `web/server.go:461` | session.create 默认 WS |
| `handleLayout` | method | `web/server.go:489` | 合并 Layout |
| `handlePlugins` | method | `web/server.go:573` | Plugin Graph |
| `handlePluginUI` | method | `web/server.go:945` | `/plugin-ui/` |
| `shellHTML`/`sdkJS`/`appFS` | var | `web/embed.go:11,14,20` | embed 静态资源 |

---

## web/static + sdk — Shell / Platform Module

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| Shell boot | js | `web/static/app/main.js:1` | 五区 host + layout/plugins/settings |
| panelHost mapping | js | `web/static/app/main.js:19-25` | top/bottom/left/center/right |
| SSE | js | `web/static/app/events.js` | 订阅 `/events` |
| Loader | js | `web/static/app/loader.js` | 插件 UI 装载 |
| Plugin graph UI | js | `web/static/app/plugins-panel.js` | Plugins 弹窗 |
| Settings UI | js | `web/static/app/settings.js` | Settings 弹窗 |
| Shell HTML | html | `web/static/shell.html` | 五区骨架 |
| `LiteAgent` | js | `sdk/lite-agent.js:1` | call/callCap/on/emit/complete |
| web copy | js | `web/static/sdk.js` | 与 sdk 同步（测试 `sdk_sync_test.go`） |

---

## render/mdansi

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| `Render` | fn | `render/mdansi/mdansi.go:30` | Markdown → ANSI |
| `Indent` | fn | `render/mdansi/mdansi.go:257` | 缩进辅助 |

---

## scripts / 构建

| 符号 | kind | path:line | 角色 |
|------|------|-----------|------|
| Windows build | script | `scripts/build.ps1` | 增量构建 |
| Unix build | script | `scripts/build.sh` | 同逻辑 |
| Shipped plugin groups | conf | `scripts/shipped-plugins.conf:6-9` | core: agent/session/llm-openai/context-manager；tools: filetools/… |
| Base layout | json | `layout.json:1-17` | main 页五区槽位 |

---

## 文档入口（辅助，非真源）

| 文档 | path | 备注 |
|------|------|------|
| 规范名词 | `CONTEXT.md` | 术语真源 |
| 模块文档索引 | `docs/modules/README.md` | 与本图 L1 一致 |
| Host 模块 | `docs/modules/serve.md` | 与 `serve/` 代码一致 |
| ADR 索引 | `CLAUDE.md` 现行结论表 | `CurrentProtocol` 以代码 `=6` 为准 |

---

## Unverified 符号

| 符号/区域 | 原因 |
|-----------|------|
| `plugins/**` 全部符号 | 用户范围排除 |
| `web/static/app/*.js` 内部函数级边 | 未做 JS 调用图；仅模块职责 |
| `dist/**` 构建产物 | 非源码 |
| `docs/archive/**` | 历史 spec，非现行契约 |
