# 核心符号索引

路径以仓库根目录为基准。行号对应当前实现。

## 1. 入口与平台

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `run` | fn | `cmd/liteagent-server/server.go:28` | 组装 Host、WebUI 和 HTTP |
| `ScanRoot` | fn | `internal/plugin/discovery.go:21` | 扫描插件 Manifest |
| `AutostartClosure` | fn | `internal/plugin/mount.go:8` | 计算自动启动闭包 |
| `Host.Run` | fn | `internal/host/host.go:197` | 创建 Host 并启动插件 |
| `Host.PluginSnapshot` | fn | `internal/host/snapshot.go:25` | 输出生命周期快照 |
| `Server.Handler` | fn | `internal/webui/server.go:74` | 组装 HTTP 路由 |
| `serveBrowserCall` | fn | `internal/webui/bridge.go:22` | Browser Call 适配 |
| `PublishState` | fn | `internal/webui/publisher.go:148` | 发布 UI State |
| `PublishCommand` | fn | `internal/webui/publisher.go:176` | 发布 UI Command |

## 2. Host 路由

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `Host.read` | fn | `internal/host/transport.go:395` | 读取插件 Frame |
| `Host.routeFrame` | fn | `internal/host/transport.go:19` | 分派 Frame 类型 |
| `Host.handleRequest` | fn | `internal/host/transport.go:50` | 处理插件请求 |
| `Host.routeOwner` | fn | `internal/host/transport.go:192` | 解析 `(capability, method)` |
| `Host.handleResponse` | fn | `internal/host/transport.go:206` | 闭环 pending |
| `Host.handleEvent` | fn | `internal/host/transport.go:239` | 转发归因事件 |
| `Host.forward` | fn | `internal/host/transport.go:307` | 转发请求到目标插件 |
| `Host.CallWithEventsInContext` | fn | `internal/host/call.go:23` | Host 发起带事件调用 |
| `Host.hostUIPublish` | fn | `internal/host/transport.go:411` | 路由 UI Push |

## 3. Plugin SDK

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `Plugin.Listen` | fn | `pluginsdk/plugin.go:143` | 插件 Frame 接收循环 |
| `Plugin.dispatch` | fn | `pluginsdk/plugin.go:198` | Handler 分派 |
| `Plugin.waitResult` | fn | `pluginsdk/plugin.go:267` | 等待 Response 和事件 |
| `Plugin.callPending` | fn | `pluginsdk/plugin.go:299` | 登记出站调用 |
| `Plugin.CallWithCallback` | fn | `pluginsdk/plugin.go:343` | 发起流式调用 |
| `Plugin.EmitWithIDInContext` | fn | `pluginsdk/plugin.go:390` | 发送归因事件 |

## 4. Agent

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `turn` | fn | `plugins/agent/turn.go:40` | 创建 Turn 并串行化 Session |
| `cancel` | fn | `plugins/agent/turn.go:86` | 取消活动 Turn |
| `runTurn` | fn | `plugins/agent/turn.go:192` | 执行主循环并收口终态 |
| `appendFacts` | fn | `plugins/agent/turn.go:113` | 写 Session Fact |
| `assembleMessages` | fn | `plugins/agent/turn.go:123` | 读取模型上下文 |
| `collectTools` | fn | `plugins/agent/turn.go:135` | 收集可选工具 |
| `callTool` | fn | `plugins/agent/turn.go:171` | 执行工具调用 |
| `streamAccumulator.apply` | fn | `plugins/agent/stream.go:97` | 累计 LLM 增量 |
| `plugin.updateStream` | fn | `plugins/agent/stream.go:171` | 更新 Session Stream |

## 5. Session

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `create` | fn | `plugins/session/store.go:204` | 创建 Session |
| `append` | fn | `plugins/session/store.go:247` | 写入 Fact 并推送 State |
| `derive` | fn | `plugins/session/store.go:379` | 投影模型上下文 |
| `updateStream` | fn | `plugins/session/ui_state.go:114` | 接收流式快照 |
| `selectSession` | fn | `plugins/session/ui_state.go:182` | 选择 UI Session |
| `state` | fn | `plugins/session/ui_state.go:211` | 重建 UI State |
| `publishAfterAppend` | fn | `plugins/session/ui_state.go:256` | 推送 Fact 变化 |
| `chatState` | fn | `plugins/session/ui_state.go:324` | 投影 ChatState |

## 6. LLM

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `complete` | fn | `plugins/llm-openai/openai.go:71` | 选择流式或 JSON 模式 |
| `completeJSON` | fn | `plugins/llm-openai/openai.go:101` | 非流式上游调用 |
| `completeStream` | fn | `plugins/llm-openai/openai.go:142` | 流式上游调用 |
| `emitJSON` | fn | `plugins/llm-openai/openai.go:269` | 发送 `llm.chunk` |

## 7. 浏览器

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `createLiteAgent.call` | fn | `internal/webui/static/lite-agent.js:54` | 发起 Browser Call |
| `createLiteAgent.onUI` | fn | `internal/webui/static/lite-agent.js:146` | 接收 UI Push |
| `session-chat.send` | component | `plugins/session/ui/main.js:827` | 发起 Turn |
| `session-chat.cancel` | component | `plugins/session/ui/main.js:848` | 取消 Turn |
| `applyBootstrap` | fn | `internal/webui/static/app.js:193` | 更新插件中心与 Mount |
| `renderPluginCenter` | fn | `internal/webui/static/app.js:161` | 渲染插件关系视图 |

## 8. 核心边

| From | To | Kind | 证据 |
|---|---|---|---|
| `liteagent-server` | `Host` | owns | `cmd/liteagent-server/server.go:41` |
| `agent` | `session` | depends_on | `plugins/agent/plugin.json:8` |
| `agent` | `llm-openai` | depends_on | `plugins/agent/plugin.json:8` |
| `agent` | `session` | calls | `plugins/agent/turn.go:114` |
| `agent` | `llm-openai` | calls | `plugins/agent/turn.go:300` |
| `llm-openai` | `agent` | emits event | `plugins/llm-openai/main.go:53` |
| `agent` | `session` | stream update | `plugins/agent/stream.go:172` |
| `session` | `WebUI` | UI Push | `plugins/session/ui_state.go:280` |
| `WebUI` | `Host` | Browser Call | `internal/webui/bridge.go:74` |

## 9. 未核实项

`tools` 插件不在当前最小组合。Agent 将 `tools.list` 和 `tools.call` 视为可选能力。

上游 API 的实际鉴权和模型行为由部署配置决定。本文只覆盖 `llm-openai` 的本地请求链路。
