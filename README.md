# LiteAgent

LiteAgent 是一套轻量、可组合的进程外插件 Runtime。Host 提供插件发现、进程管理、Capability 路由和 WebUI 平台壳。具体业务由独立插件提供。

Host 与 Plugin 是两个独立层次。项目整体文档只定义平台契约，不把任何业务插件视为内核组成部分。

## 当前状态

当前代码具备 Host、pluginsdk、Frame 协议、Manifest 发现、Provider 生命周期、执行上下文、取消和 Frame Priority 流水线。`plugins/` 当前发布 `agent`、`llm-openai`、`session`、`model-context`、`tools`、`file-tools` 和 `shell-tools` 七个插件。

Workspace Registry 由 Session 插件拥有。Session 创建后绑定 Workspace；历史列表按 Workspace 过滤，多会话标签按 Workspace 恢复。Agent 从 Session 解析规范 Workspace，拒绝调用方传入的不一致路径。

model-context 把 system prompt 与 runtime context 写成 Session Facts，再投影实际模型输入；关闭该插件时，agent 基线直接进入 llm-openai。

消费链上的缺席即放行。插件声明了 `consumes` 但没有活跃实例时，Host 跳过这一跳，调用直接交给该键的 Provider。平台不提供强制拦截。

Sandbox、Web 工具和 Subagent 插件已从当前源码移除，代码保留在 git 历史。设计稿保留在 [docs/design/plugins/README.md](docs/design/plugins/README.md)，作为路线图。

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

## 文档地图

文档归属的完整规则见 [CONSTRAINT.md](CONSTRAINT.md)。下表是全部入口。

### 元文档

| 文档 | 内容 |
|---|---|
| [CONTEXT.md](CONTEXT.md) | 平台术语 |
| [CONSTRAINT.md](CONSTRAINT.md) | 行为约束与文档归属 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Host 与 Plugin 的总体结构 |
| [PRESSURE.md](PRESSURE.md) | 测试规程与压力场景 |

### 平台契约

| 文档 | 内容 |
|---|---|
| [docs/platform/protocol.md](docs/platform/protocol.md) | Host 与 Plugin 的 Frame 协议 |
| [docs/platform/pluginsdk.md](docs/platform/pluginsdk.md) | 插件开发接口 |
| [docs/platform/manifest.md](docs/platform/manifest.md) | Manifest、UI 和 Settings 静态契约 |
| [docs/platform/webui.md](docs/platform/webui.md) | Host WebUI 和 Browser Bridge |
| [docs/platform/plugin-dev.md](docs/platform/plugin-dev.md) | 插件包结构和开发流程 |
| [docs/platform/build.md](docs/platform/build.md) | 构建流程、增量缓存和 dist 产物布局 |
| [docs/platform/development.md](docs/platform/development.md) | 平台模块、结构体和图解 |

### 目标契约

| 文档 | 内容 |
|---|---|
| [docs/design/composition-model-context-tools-sandbox.md](docs/design/composition-model-context-tools-sandbox.md) | 消费链组合方案 |
| [docs/design/plugins/README.md](docs/design/plugins/README.md) | 插件设计稿索引、接缝全景与插件地图 |
| [docs/design/webui-long-session-performance.md](docs/design/webui-long-session-performance.md) | 长会话性能设计与实现 |

### Prompt 契约

| 文档 | 内容 |
|---|---|
| [docs/prompts/README.md](docs/prompts/README.md) | Prompt 契约与写作原则 |

### 插件自身

| 文档 | 内容 |
|---|---|
| [plugins/README.md](plugins/README.md) | 插件清单和实现状态 |
| `plugins/<name>/README.md` | 用途、配置、依赖和状态 |
| `plugins/<name>/communication.md` | 方法、payload、状态、前端交互 |

### 导航、过程与参考

| 文档 | 内容 |
|---|---|
| [docs/README.md](docs/README.md) | docs 目录索引 |
| [docs/reference/project-map/OVERVIEW.md](docs/reference/project-map/OVERVIEW.md) | 运行图与链路 |
| [docs/reference/project-map/INDEX.md](docs/reference/project-map/INDEX.md) | 核心符号索引 |
| [docs/reference/project-map/DEPENDENCY.md](docs/reference/project-map/DEPENDENCY.md) | 依赖与调用图 |
| [docs/archive/progress/README.md](docs/archive/progress/README.md) | 过程记录索引与状态标注 |
| `docs/research/` | 外部参考材料 |

`docs/reference/` 是易失导航，只指路不定事实。`docs/archive/` 是历史记录，不是规范。
项目整体文档提到具体插件时，只能作为非核心示例。插件是否可用、如何通信，以该插件自己的文档为准。

## 核心原则

- 路由只看 `(capability, method)`；
- 插件名只用于身份、日志和观测；
- 插件之间不直连；
- 缺席即放行，平台不提供强制拦截；
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
internal/webui/      WebUI Shell、Browser Bridge 和静态资源
pluginsdk/           插件开发入口
protocol/            Host 与 Plugin 线契约
plugins/             独立插件源码
docs/                平台契约、设计稿、Prompt 契约与过程记录
scripts/             构建脚本
```
