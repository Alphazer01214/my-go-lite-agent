状态：当前实现

# AGENT

本文介绍项目根目录下的 Markdown 文档。
每份文档定义一类事实。读错文档会得到过期或越界的答案。
事实归属以 [CONSTRAINT.md](CONSTRAINT.md) 为准。

## 1. 文档一览

| 文档 | 定义什么 | 何时读 |
|---|---|---|
| [CONSTRAINT.md](CONSTRAINT.md) | 协作规则、文档归属、文档检查、测试流程 | 任何改动动手前 |
| [CONTEXT.md](CONTEXT.md) | 平台术语，含每个术语的 Avoid 用法 | 命名、写文档、提到平台概念前 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 平台结构、Frame 路由、消息边界、目录映射 | 改 `protocol`、`pluginsdk`、`internal/`、`cmd/` 前 |
| [PRESSURE.md](PRESSURE.md) | 测试规程、数据档位、压力与错误条目 | 跑可用性、压力或错误场景前 |
| [AGENT.md](AGENT.md) | 本文，根目录文档导读 | 不确定事实归哪份文档时 |

## 2. 边界

根目录文档只定义平台级事实。
插件契约写在 `plugins/<name>/`，跨插件方案写在 `docs/design/`。
其余内容的归属见 [CONSTRAINT.md](CONSTRAINT.md) 的文档归属表，本文不重复。
