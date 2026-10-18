# 核心符号索引

路径以仓库根目录为基准。行号对应当前实现。

## 1. 入口与平台

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `run` | fn | `cmd/liteagent-server/server.go:25` | 组装 Host、WebUI 和 HTTP |
| `ScanRoot` | fn | `internal/plugin/discovery.go:21` | 扫描插件 Manifest |
| `AutostartClosure` | fn | `internal/plugin/mount.go:8` | 计算自动启动闭包 |
| `Host.Run` | fn | `internal/host/host.go:210` | 创建 Host 并启动插件 |
| `Host.PluginSnapshot` | fn | `internal/host/snapshot.go:46` | 输出生命周期快照 |
| `Server.Handler` | fn | `internal/webui/server.go:74` | 组装 HTTP 路由 |
| `serveBrowserCall` | fn | `internal/webui/bridge.go:23` | Browser Call 适配 |
| `PublishState` | fn | `internal/webui/publisher.go:148` | 发布 UI State |
| `PublishCommand` | fn | `internal/webui/publisher.go:176` | 发布 UI Command |

## 2. Host 路由

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `Host.read` | fn | `internal/host/transport.go:482` | 读取插件 Frame |
| `Host.routeFrame` | fn | `internal/host/transport.go:21` | 分派 Frame 类型 |
| `Host.handleRequest` | fn | `internal/host/transport.go:54` | 处理插件请求 |
| `Host.routeOwner` | fn | `internal/host/transport.go:254` | 解析 `(capability, method)` |
| `Host.handleResponse` | fn | `internal/host/transport.go:290` | 闭环 pending |
| `Host.handleEvent` | fn | `internal/host/transport.go:325` | 转发归因事件 |
| `Host.forwardPipeline` | fn | `internal/host/transport.go:394` | 沿流水线转发请求 |
| `Host.selectPipelineCandidate` | fn | `internal/host/pipeline.go:69` | 选择下一个 Consumer |
| `Host.hostProceed` | fn | `internal/host/pipeline.go:127` | 推进 Frame Priority 流水线 |
| `Host.CallWithEventsInContext` | fn | `internal/host/call.go:25` | Host 发起带事件调用 |
| `Host.hostUIPublish` | fn | `internal/host/transport.go:498` | 路由 UI Push |

## 3. Plugin SDK

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `Plugin.Listen` | fn | `pluginsdk/plugin.go:195` | 插件 Frame 接收循环 |
| `Plugin.dispatch` | fn | `pluginsdk/plugin.go:257` | Handler 分派 |
| `Plugin.waitResult` | fn | `pluginsdk/plugin.go:377` | 等待 Response 和事件 |
| `Plugin.callPendingInContext` | fn | `pluginsdk/plugin.go:426` | 登记出站调用 |
| `Request.Proceed` | fn | `pluginsdk/plugin.go:28` | 继续 Frame Priority 流水线 |
| `Plugin.Call` | fn | `pluginsdk/plugin.go:471` | 发起 Context Call |
| `Plugin.CallInContext` | fn | `pluginsdk/plugin.go:485` | 在 Execution Context 中调用 |
| `Plugin.CallWithCallback` | fn | `pluginsdk/plugin.go:480` | 发起流式调用 |
| `Plugin.EmitWithIDInContext` | fn | `pluginsdk/plugin.go:527` | 发送归因事件 |

## 4. Agent

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `turn` | fn | `plugins/agent/turn.go:93` | 创建 Turn 并串行化 Session |
| `cancel` | fn | `plugins/agent/turn.go:139` | 取消活动 Turn |
| `runTurn` | fn | `plugins/agent/turn.go:259` | 执行主循环并收口终态 |
| `appendFacts` | fn | `plugins/agent/turn.go:180` | 写 Session Fact |
| `assembleMessages` | fn | `plugins/agent/turn.go:190` | 读取模型上下文 |
| `collectTools` | fn | `plugins/agent/turn.go:202` | 收集可选工具 |
| `callTool` | fn | `plugins/agent/turn.go:238` | 执行工具调用 |
| `streamAccumulator.apply` | fn | `plugins/agent/stream.go:81` | 累计 LLM 增量 |
| `plugin.updateStream` | fn | `plugins/agent/stream.go:155` | 更新 Session Stream |

## 5. Session

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `create` | fn | `plugins/session/store.go:249` | 创建 Session |
| `append` | fn | `plugins/session/store.go:292` | 写入 Fact 并推送 State |
| `derive` | fn | `plugins/session/store.go:424` | 投影模型上下文 |
| `updateStream` | fn | `plugins/session/ui_state.go:86` | 接收流式快照 |
| `selectSession` | fn | `plugins/session/ui_state.go:157` | 选择 UI Session |
| `state` | fn | `plugins/session/ui_state.go:186` | 重建 UI State |
| `publishAfterAppend` | fn | `plugins/session/ui_state.go:227` | 推送 Fact 变化 |
| `chatState` | fn | `plugins/session/ui_state.go:295` | 投影 ChatState |

## 6. LLM

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `complete` | fn | `plugins/llm-openai/openai.go:72` | 选择流式或 JSON 模式 |
| `completeJSON` | fn | `plugins/llm-openai/openai.go:109` | 非流式上游调用 |
| `completeStream` | fn | `plugins/llm-openai/openai.go:150` | 流式上游调用 |
| `emitJSON` | fn | `plugins/llm-openai/openai.go:274` | 发送 `llm.chunk` |

## 7. Tool 与上下文插件

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `toolsPlugin.register` | fn | `plugins/tools/main.go` | 注册 Tool |
| `toolsPlugin.list` | fn | `plugins/tools/main.go` | 过滤并列出 Tool |
| `toolsPlugin.call` | fn | `plugins/tools/main.go` | 分发 Tool Call |
| `registry.put` | fn | `plugins/tools/registry.go` | 维护唯一 Tool 名称 |
| `plugin.build` | fn | `plugins/model-context/builder.go` | 构建 Model Context |
| `plugin.compact` | fn | `plugins/model-context/compact.go` | 写入上下文摘要 |
| `sandbox.check` | fn | `plugins/sandbox/main.go` | 执行策略并 Proceed |
| `policyStore.decide` | fn | `plugins/sandbox/policy.go` | 匹配风险规则 |
| `plugin.execute` | fn | `plugins/shell-tools/main.go` | 执行进程 |
| `fetchWithClient` | fn | `plugins/web-tools/fetch.go` | 受限 HTTP 读取 |
| `plugin.run` | fn | `plugins/subagent/main.go` | 运行同步子 Agent |

## 8. 浏览器

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `createLiteAgent.call` | fn | `internal/webui/static/lite-agent.js:54` | 发起 Browser Call |
| `createLiteAgent.onUI` | fn | `internal/webui/static/lite-agent.js:146` | 接收 UI Push |
| `session-chat.send` | component | `plugins/session/ui/main.js:827` | 发起 Turn |
| `session-chat.cancel` | component | `plugins/session/ui/main.js:848` | 取消 Turn |
| `applyBootstrap` | fn | `internal/webui/static/app.js:210` | 更新插件中心与 Mount |
| `renderPluginCenter` | fn | `internal/webui/static/app.js:174` | 渲染插件关系视图 |

## 9. 核心边

| From | To | Kind | 证据 |
|---|---|---|---|
| `liteagent-server` | `Host` | owns | `cmd/liteagent-server/server.go:41` |
| `agent` | `session` | depends_on | `plugins/agent/plugin.json:8` |
| `agent` | `llm-openai` | depends_on | `plugins/agent/plugin.json:8` |
| `agent` | `tools` | depends_on | `plugins/agent/plugin.json` |
| `agent` | `model-context` | depends_on | `plugins/agent/plugin.json` |
| `agent` | `session` | calls | `plugins/agent/turn.go:169` |
| `agent` | `llm-openai` | calls | `plugins/agent/turn.go:373` |
| `agent` | `tools` | calls | `plugins/agent/turn.go` |
| `agent` | `model-context` | calls | `plugins/agent/turn.go` |
| `tools` | Tool Provider | calls | `plugins/tools/main.go` |
| `sandbox` | `tools.call` | consumes | `plugins/sandbox/plugin.json` |
| `subagent` | `agent` | calls | `plugins/subagent/main.go` |
| `llm-openai` | `agent` | emits event | `plugins/llm-openai/main.go:60` |
| `agent` | `session` | stream update | `plugins/agent/stream.go:155` |
| `session` | `WebUI` | UI Push | `plugins/session/ui_state.go:292` |
| `WebUI` | `Host` | Browser Call | `internal/webui/bridge.go:75` |

## 10. 未核实项

上游 API 的实际鉴权和模型行为由部署配置决定。本文只覆盖 `llm-openai` 的本地请求链路。
