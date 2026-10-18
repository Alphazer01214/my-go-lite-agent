# LiteAgent

LiteAgent 是一套轻量、可组合的进程外插件 Runtime。Host 提供插件发现、进程管理、Capability 路由和 WebUI 平台壳。具体业务由独立插件提供。

Host 与 Plugin 是两个独立层次。项目整体文档只定义平台契约，不把任何业务插件视为内核组成部分。

## 当前状态

当前代码具备 Host、pluginsdk、Frame 协议、Manifest 发现、Provider 生命周期、执行上下文、取消和 Frame Priority 流水线。`plugins/` 提供可发布的 Agent、Session、LLM、Model Context、Tools、Sandbox、File、Shell、Web 和 Subagent 组合。

WebUI 平台契约已形成文档。具体插件的 UI 和通信契约只写在对应插件目录。

## 快速开始

运行平台测试：

```powershell
go test ./...
```

构建 Host：

```powershell
go build ./cmd/liteagent-server
```

构建可分发的 dist 布局（增量构建，支持 `--clean`、`--verify`、`--only`）：

```powershell
powershell -File scripts/build.ps1 -Verify
```

Host 启动时按以下顺序查找插件目录：

1. 可执行文件旁的 `plugins/`；
2. 当前工作目录下的 `plugins/`。

插件配置由插件自己读写。项目文档不定义插件业务配置。

## 文档边界

| 文档范围 | 位置 | 内容 |
|---|---|---|
| Host 平台 | 根目录、`docs/` | 架构、Frame、SDK、Manifest、WebUI |
| 插件自身 | `plugins/<name>/` | 方法、payload、状态、前端交互 |
| 插件目录 | `plugins/README.md` | 插件清单和实现状态 |

项目整体文档提到具体插件时，只能作为非核心示例。插件是否可用、如何通信，以该插件自己的文档为准。

## 平台文档

| 文档 | 内容 |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Host 与 Plugin 的总体结构 |
| [docs/development.md](docs/development.md) | 平台模块、结构体和图解 |
| [docs/protocol.md](docs/protocol.md) | Host 与 Plugin 的 Frame 协议 |
| [docs/pluginsdk.md](docs/pluginsdk.md) | 插件开发接口 |
| [docs/manifest.md](docs/manifest.md) | Manifest、UI 和 Settings 静态契约 |
| [docs/webui.md](docs/webui.md) | Host WebUI 和 Browser Bridge |
| [docs/plugin-dev.md](docs/plugin-dev.md) | 插件包结构和开发流程 |
| [docs/build.md](docs/build.md) | 构建流程、增量缓存和 dist 产物布局 |

## 核心原则

- 路由只看 `(capability, method)`；
- 插件名只用于身份、日志和观测；
- 插件之间不直连；
- Host 不实现业务面板；
- Plugin Runtime 驱动自己拥有的 Component；
- UI State 不替代插件自己的持久化；
- 插件私有结构体不得直接成为平台契约；
- 平台文档与插件文档分别维护。

## 项目结构

```text
cmd/                 Host 可执行入口
internal/host/       进程管理与 Frame 路由
internal/plugin/     Manifest 发现
pluginsdk/           插件开发入口
protocol/            Host 与 Plugin 线契约
plugins/             独立插件源码与原型
docs/                平台契约和开发文档
scripts/             构建脚本
```
