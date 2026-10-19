# 插件目录

`plugins/` 是 LiteAgent 的独立扩展层。插件不是 Host 的组成部分。每个可挂载插件必须包含 `main.go` 和 `plugin.json`。

## 1. 当前源码

| 插件 | 状态 | 主要能力 |
|---|---|---|
| `agent` | 当前实现 | Turn、Step、取消；Tools 和 Model Context 缺失时降级 |
| `llm-openai` | 当前实现 | OpenAI-compatible Completion |
| `session` | 当前实现 | Session Fact Log、Workspace Registry、投影和多会话 UI |

`tools`、`model-context`、`sandbox`、`file-tools`、`shell-tools`、`web-tools` 和 `subagent` 曾在本仓库实现，随后从源码移除，代码保留在 git 历史。

## 2. 跨插件通信

跨插件 JSON 形状由提供方插件的 `communication.md` 定义。

调用方在自己的插件目录内维护适配类型。平台目录不保存插件业务消息。

## 3. 文档要求

每个插件包含：

- `README.md`：用途、配置、依赖和状态；
- `communication.md`：方法、payload、Event、调用图和前端交互。

插件通信文档必须区分：

- 当前代码契约；
- WebUI 目标契约；
- 尚未实现的方法。

## 4. WebUI 归属

`session` 提供当前全部顶层业务 Component：

| Component | Slot | 属主 |
|---|---|---|
| `session-history` | left | session |
| `session-chat` | center | session |
| `session-trace` | right | session |

`agent` 只提供 Settings。`llm-openai` 只提供 Settings。两者不直接渲染聊天正文。

## 5. 当前组合

```text
agent
├── llm-openai
└── session
```

启动顺序由 Manifest `autostart` 和 `depends_on` 决定。`agent` 负责 Turn，`llm-openai` 负责模型调用，`session` 负责持久化和 WebUI。

`agent` 对 `tools` 和 `model-context` 是弱依赖。能力缺失时跳过工具阶段，或改用 `session.derive` 构建模型输入。

## 6. 未实现能力

Tools、Model Context、Sandbox 和文件、Shell、Web、Subagent 插件不在当前源码。交互式终端也未实现。

## 7. 构建清单

`scripts/shipped-plugins.conf` 是发布目标的唯一清单，`scripts/build.sh` 和 `scripts/build.ps1` 共用。当前收录：

```text
core: agent session llm-openai
tools:（空）
```

`core` 是 autostart 根插件。`tools` 留给后续的非 autostart Tool Provider。清单只收录 `plugins/<name>/` 下已有 `plugin.json` 的插件。没有 `plugin.json` 的目录不是可挂载插件，构建脚本会跳过并告警，不会进入 dist。

后续新增插件时，再同步修改清单和本目录。构建流程见 [docs/build.md](../docs/build.md)。
