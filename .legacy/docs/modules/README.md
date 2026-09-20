# 模块开发文档索引

本目录是 **plugins/ 以外** 各代码模块的开发文档。规范名词以 [CONTEXT.md](../../CONTEXT.md) 为准；架构决策以 [docs/adr/](../adr/) 为准；Host↔Plugin 线协议总览见 [docs/protocol.md](../protocol.md)。

历史 feature spec 已归档于 [docs/archive/](../archive/)，**不是现行契约**。

## 模块地图

```text
                    ┌─────────────────────────────────────┐
                    │  cmd/liteagent-cli   cmd/liteagent-server │
                    └───────────┬─────────────┬───────────┘
                                │             │
                    ┌───────────▼─────────────▼───────────┐
                    │            internal/app             │
                    │  (Medium 装配 · REPL · Command · Web 启动) │
                    └──────┬──────────┬──────────┬────────┘
                           │          │          │
              ┌────────────▼──┐  ┌────▼────┐  ┌──▼──────────┐
              │ serve (Host)  │  │ web/    │  │ layout/     │
              │ L0 内核       │  │ Shell   │  │ assembly/   │
              └───────┬───────┘  │ SSE/SDK │  │ discovery/  │
                      │          └─────────┘  └─────────────┘
         ┌────────────┼────────────┐
         │            │            │
   ┌─────▼────┐ ┌─────▼────┐ ┌─────▼─────┐
   │ protocol │ │ pluginsdk│ │ plugin    │
   │ Frame 编解码 │ │ 作者 SDK │ │ Manifest  │
   └──────────┘ └──────────┘ └───────────┘
                      │
              render/mdansi · sdk/lite-agent.js · scripts/
```

## 文档列表

| 模块 | 路径 | 一句话职责 |
|------|------|------------|
| [protocol](protocol.md) | `protocol/` | Frame 线格式编解码（`uint32` + JSON） |
| [plugin](plugin.md) | `plugin/` | `plugin.json` Manifest 类型与校验 |
| [pluginsdk](pluginsdk.md) | `pluginsdk/` | 插件作者 API + 共享契约类型 |
| [discovery](discovery.md) | `discovery/` | 扫描插件目录，得到「机器上有什么」 |
| [assembly](assembly.md) | `assembly/` | 挂载计划：Autostart + dependsOn 闭包；UI 裁决 |
| [layout](layout.md) | `layout/` | Web Medium 页面/槽位几何真源 |
| [serve](serve.md) | `serve/` | Host L0 内核：进程生命周期 + 按插件名转发 |
| [internal/app](internal-app.md) | `internal/app/` | 双入口共享 Medium 运行时 |
| [cmd/liteagent-cli](cmd-liteagent-cli.md) | `cmd/liteagent-cli/` | CLI 入口 + 进程边界集成测试 |
| [cmd/liteagent-server](cmd-liteagent-server.md) | `cmd/liteagent-server/` | Web 入口 |
| [web](web.md) | `web/` + `web/static/` | Web Medium：HTTP 面 + Shell + 插件 UI 装载 |
| [sdk](sdk.md) | `sdk/` | Platform Module（`lite-agent.js` 作者 SDK） |
| [render/mdansi](render-mdansi.md) | `render/mdansi/` | Markdown → ANSI（CLI Medium 绘制） |
| [scripts](scripts.md) | `scripts/` | 跨平台构建与出厂插件清单 |

## 分层红线（ADR-0030 / ADR-0026）

| 层 | 允许内容 | Host（`serve/`） | Medium（`web/`、`internal/app/` 运行时面） |
|----|----------|------------------|---------------------------------------------|
| **L0** | 按插件名转发、进程生命周期、通用挂载、通用事件/应答、hostFaces、`host.ensurePlugins` | 应拥有 | 只经 Host 使用 |
| **L1** | 能力名与方法、payload 领域字段 | 不得调用/解析/出现常量名 | Go 运行时不得出现；绘制可用 pluginsdk 展示类型 |
| **L2** | 插件目录名字面量（调度硬编码） | 绝不能知道 | 同理 |

**Deferred 例外**：`agent.request` / `agent.inject` / `agent.confirm` 暂由 Host 特例处理；重开前勿扩大该面。

## 启动数据流

```text
discovery.Scan(pluginsDir)
  → assembly.ResolveAutostart(res)          // Autostart 根 + dependsOn 闭包
  → serve.Start(plan.Mounted) + SetCatalog  // Host 启动；目录供 ensurePlugins
  → [Web] layout.Load + Merge(ui.pages) + ResolveUIMounts
  → Medium 注册 Subscribe / RegisterApproval / CommandPlane
```

运行中懒挂载：Agent Scheme `dependsPlugins` → Frame `host.ensurePlugins` → `assembly.ResolveClosure`（幂等，软失败）。

## 改代码前必读

- 改 Host / Medium / 路由 → ADR-0030 → ADR-0026 → ADR-0027
- 改插件 / Manifest / UI 面 → [plugins/README.md](../../plugins/README.md) → ADR-0027 → ADR-0012
- 改装配 / scheme / 挂载 → ADR-0021 → ADR-0023 → ADR-0022
- 破坏 Manifest/UI 契约 → bump `plugin.CurrentProtocol`；破坏 Frame 线格式 → bump `protocol.Version`
- Shell 宿主槽 = `top|bottom|left|center|right`（ADR-0031）；session 占 left+center，chat/trace 由 session 组件提供
