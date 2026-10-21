# 插件目录

`plugins/` 是 LiteAgent 的独立扩展层。插件不是 Host 的组成部分。每个可挂载插件必须包含 `main.go` 和 `plugin.json`。

## 1. 当前源码

| 插件 | 状态 | 主要能力 |
|---|---|---|
| `agent` | 当前实现 | Turn、Step、取消；Tools 缺失时降级 |
| `llm-openai` | 当前实现 | OpenAI-compatible Completion |
| `session` | 当前实现 | Session Fact Log、Workspace Registry、投影和多会话 UI |
| `model-context` | 当前实现 | `llm.complete` 链上组装；system / runtime Facts、快照与确定性压缩 |
| `tools` | 当前实现 | Tool 注册、Generation 感知发现与分发 |
| `file-tools` | 当前实现 | Workspace 文件读写八件套 |
| `shell-tools` | 当前实现 | 同步命令执行与进程树取消 |
| `sandbox` | 当前实现 | 工具调用链上的权限裁决：allow、modify、deny |
| `subagent` | 当前实现 | 单工具委派：阻塞执行子会话，最终回答作为工具结果 |

`web-tools` 曾在本仓库设计，随后从源码移除。

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
| `session-dock` | bottom | session |

`agent` 只提供 Settings。`llm-openai` 只提供 Settings。两者不直接渲染聊天正文。

## 5. 当前组合

```text
agent
├── llm-openai
├── model-context（llm.complete 消费者）
├── session
├── tools
│   ├── file-tools（file）
│   └── shell-tools（shell）
└── subagent（loop.turn 调用方）

sandbox（可选件，autostart: false）
└── 消费 tools.call、file.write、file.replace、file.patch、shell.execute
```

启动顺序由 Manifest `autostart` 和 `depends_on` 决定。`agent` 负责 Turn，`model-context` 负责把 system / runtime Facts 写入 Session 并投影模型输入，`llm-openai` 负责模型调用，`session` 负责持久化和 WebUI。

`tools` 是通用注册表与分发器。`file-tools` 和 `shell-tools` 在启动时通过 `tools.sync` 注册工具；tools 只按 `(capability, method)` 分发，不直连 Provider。

`agent` 对 `tools` 和 `model-context` 是弱依赖。缺少 model-context 时，agent 的 `session.derive + tools.list` 基线直接进入 llm-openai；缺少 tools 时跳过工具阶段。

`sandbox` 是可选件。它的 `autostart` 是 `false`，从插件中心启动后进链，在链上时裁决 `tools.call` 与直连高危能力。缺席即放行，缺席是部署约定。

`subagent` 通过 `tools.sync` 注册单个 `subagent` 工具。模型调用后，插件创建子会话、阻塞执行 `loop.turn`，并把子代理最终回答作为工具结果返回。子会话 prompt 由 model-context 的 `subagent_sections` 管理；父会话记录 `subagent_spawned` / `subagent_finished`，聊天区提供只读子会话入口。

## 6. 未实现能力

Web 工具不在当前源码。交互式终端和后台 Shell 也未实现。sandbox 的确认流、确认面板与 Prompt 贡献也未实现。subagent 暂不支持后台/续聊、嵌套和独立取消。

## 7. 构建清单

`scripts/shipped-plugins.conf` 是发布目标的唯一清单，`scripts/build.sh` 和 `scripts/build.ps1` 共用。当前收录：

```text
core: agent session llm-openai model-context tools file-tools shell-tools subagent
tools:（空）
optional: sandbox
```

`core` 是 autostart 根插件。`tools` 留给后续的非 autostart Tool Provider。`optional` 是随发行版交付但不随 Host 启动的插件。清单只收录 `plugins/<name>/` 下已有 `plugin.json` 的插件。没有 `plugin.json` 的目录不是可挂载插件，构建脚本会跳过并告警，不会进入 dist。

后续新增插件时，再同步修改清单和本目录。构建流程见 [docs/platform/build.md](../docs/platform/build.md)。
