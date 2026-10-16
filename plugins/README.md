# 插件目录

`plugins/` 是 LiteAgent 的独立扩展层。插件不是 Host 的组成部分。每个可挂载插件必须包含 `main.go` 和 `plugin.json`。

## 1. 当前源码

| 插件 | 状态 | 主要能力 |
|---|---|---|
| `agent` | 当前实现 | Turn、Step、工具循环、取消 |
| `llm-openai` | 当前实现 | OpenAI-compatible Completion |
| `session` | 当前实现 | Session Fact Log 和投影 |
| `context-manager` | 原型 | 未形成可挂载插件 |
| `filetools` | 空目录 | 未实现 |

## 2. 文档要求

每个插件包含：

- `README.md`：用途、配置、依赖和状态；
- `communication.md`：方法、payload、Event、调用图和前端交互。

插件通信文档必须区分：

- 当前代码契约；
- WebUI 目标契约；
- 尚未实现的方法。

## 3. WebUI 归属

当前只有 `session` 提供顶层业务 Component：

| Component | Slot | 属主 |
|---|---|---|
| `session-history` | left | session |
| `session-chat` | center | session |
| `session-trace` | right | session |

`agent` 只提供 Settings。`llm-openai` 只提供 Settings。两者不直接渲染聊天正文。

## 4. 最小组合

当前最小运行组合是：

```text
agent + llm-openai + session
```

启动顺序由 Manifest `autostart` 和 `depends_on` 决定。`agent` 负责 Turn，`llm-openai` 负责模型调用，`session` 负责持久化和 WebUI。

最小组合不加载工具插件和 context-manager。工具能力为空时，Agent 跳过工具阶段。

## 5. 构建清单

`scripts/shipped-plugins.conf` 当前列出的插件范围大于当前源码范围。实现或清理构建目标时，应同步修改脚本和本目录，不把空目录当成可运行插件。
