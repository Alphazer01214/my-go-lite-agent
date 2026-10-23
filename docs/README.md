# docs

本目录是 LiteAgent 的平台文档。目录按用途分，不按主题分。

每个目录的文档有自己的判定标准。放错目录的文档会被读者当成错误依据。

英文版核心文档见 [en/](en/)。中文文档仍是规范源。

## 1. 目录

| 目录 | 用途 | 判定标准 |
|---|---|---|
| [platform/](platform/) | 平台契约 | 照着它能实现 |
| [design/](design/) | 目标契约 | 照着它能规划 |
| [prompts/](prompts/) | Prompt 契约 | 独立体系，自成一类 |
| [archive/](archive/) | 过程记录 | 写完就冻结，只增不改，不作为参考 |
| [research/](research/) | 参考材料 | 除非指定，否则不打开。外部资料，不作为依据 |

## 2. 平台契约

照着这些文档能实现 Host 与 pluginsdk。

| 文档 | 内容 |
|---|---|
| [platform/protocol.md](platform/protocol.md) | Host 与 Plugin 的 Frame 协议 |
| [platform/pluginsdk.md](platform/pluginsdk.md) | 插件开发接口 |
| [platform/manifest.md](platform/manifest.md) | Manifest、UI 和 Settings 静态契约 |
| [platform/webui.md](platform/webui.md) | Host WebUI 和 Browser Bridge |
| [platform/plugin-dev.md](platform/plugin-dev.md) | 插件包结构和开发流程 |
| [platform/plugin-standard.md](platform/plugin-standard.md) | 插件目录组织、文件划分和拆分轴 |
| [platform/build.md](platform/build.md) | 构建流程、增量缓存和 dist 产物布局 |
| [platform/development.md](platform/development.md) | 平台分层、跨包约定和进程入口 |

平台实现规格按包划分，在 [platform/internal/](platform/internal/README.md)：

| 文档 | 内容 |
|---|---|
| [platform/internal/host.md](platform/internal/host.md) | `internal/host` 的路由、流水线、pending、取消、上下文和实例 |
| [platform/internal/plugin.md](platform/internal/plugin.md) | `internal/plugin` 的 Manifest 加载、诊断、扫描和闭包 |
| [platform/internal/webui.md](platform/internal/webui.md) | `internal/webui` 的 Registry、Bridge、UI Push 和浏览器脚本 |

## 3. 目标契约

照着这些文档能规划下一轮实现。内容可以比实现走得远，但每处都要标状态。

| 文档 | 内容 |
|---|---|
| [design/composition-model-context-tools-sandbox.md](design/composition-model-context-tools-sandbox.md) | 消费链组合方案 |
| [design/webui-long-session-performance.md](design/webui-long-session-performance.md) | 长会话性能设计与实现 |
| [design/plugins/README.md](design/plugins/README.md) | 插件设计稿索引、接缝全景与插件地图 |

## 4. Prompt 契约

| 文档 | 内容 |
|---|---|
| [prompts/README.md](prompts/README.md) | Prompt 契约与写作原则 |



## 5. 过程记录

写完就冻结。废止时在状态标注里写清取代者，不删除文件。

| 文档 | 内容 |
|---|---|
| [archive/progress/README.md](archive/progress/README.md) | 过程记录索引与五档状态标注 |

## 6. 参考材料

外部资料。不是本项目的依据，不参与契约冲突的裁决。

| 文档 | 内容 |
|---|---|
| `research/cordis-paper.md` | Cordis 论文。除非必要不打开 |

## 7. 归属与纪律

文档归属的完整规则见 [CONSTRAINT.md](../CONSTRAINT.md)。平台文档与插件文档分别维护。
插件业务契约写在 `plugins/<name>/`，不在本目录。
