# 核心符号索引

路径以仓库根目录为基准。行号对应当前实现。

## 1. 入口与平台

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `run` | fn | `cmd/liteagent-server/server.go:27` | 组装 Host、WebUI 和 HTTP |
| `ScanRoot` | fn | `internal/plugin/discovery.go:21` | 扫描插件 Manifest |
| `AutostartClosure` | fn | `internal/plugin/mount.go:8` | 计算自动启动闭包 |
| `Host.New` | fn | `internal/host/host.go:209` | 构造 Host，暂不启动插件 |
| `Host.Start` | fn | `internal/host/host.go:231` | 按自动启动闭包挂载插件 |
| `Host.PluginSnapshot` | fn | `internal/host/snapshot.go:46` | 输出生命周期快照 |
| `Server.Handler` | fn | `internal/webui/server.go:102` | 组装 HTTP 路由 |
| `Server.servePickDirectory` | fn | `internal/webui/picker.go:44` | 打开 Host 目录选择器 |
| `pickDirectoryNative` | fn | `internal/webui/picker.go:80` | 按平台调用系统文件夹选择器 |
| `serveBrowserCall` | fn | `internal/webui/bridge.go:23` | Browser Call 适配 |
| `PublishState` | fn | `internal/webui/publisher.go:155` | 发布 UI State |
| `PublishCommand` | fn | `internal/webui/publisher.go:187` | 发布 UI Command |

## 2. Host 路由

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `Host.read` | fn | `internal/host/transport.go:507` | 读取插件 Frame |
| `Host.routeFrame` | fn | `internal/host/transport.go:21` | 分派 Frame 类型 |
| `Host.handleRequest` | fn | `internal/host/transport.go:54` | 处理插件请求 |
| `Host.routeOwner` | fn | `internal/host/transport.go:278` | 解析 `(capability, method)` |
| `Host.handleResponse` | fn | `internal/host/transport.go:318` | 闭环 pending |
| `Host.handleEvent` | fn | `internal/host/transport.go:350` | 转发归因事件 |
| `Host.forwardPipeline` | fn | `internal/host/transport.go:419` | 沿流水线转发请求 |
| `Host.selectPipelineCandidate` | fn | `internal/host/pipeline.go:78` | 选择下一个 Consumer |
| `Host.hostProceed` | fn | `internal/host/pipeline.go:149` | 校验并异步推进 Frame Priority 流水线 |
| `Host.finishProceed` | fn | `internal/host/pipeline.go:196` | 在 Host 接收循环之外等待下游流水线 |
| `Host.CallWithEventsInContext` | fn | `internal/host/call.go:25` | Host 发起带事件调用 |
| `Host.hostUIPublish` | fn | `internal/host/transport.go:523` | 路由 UI Push |

## 3. Plugin SDK

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `Plugin.Serve` | fn | `pluginsdk/plugin.go:192` | 先起接收循环，再上报注册 |
| `Plugin.Listen` | fn | `pluginsdk/plugin.go:225` | 插件 Frame 接收循环 |
| `Plugin.dispatch` | fn | `pluginsdk/plugin.go:295` | Handler 分派 |
| `Plugin.waitResult` | fn | `pluginsdk/plugin.go:450` | 等待 Response 和事件 |
| `Plugin.callPendingInContext` | fn | `pluginsdk/plugin.go:503` | 登记出站调用 |
| `Request.Proceed` | fn | `pluginsdk/plugin.go:29` | 继续 Frame Priority 流水线 |
| `Plugin.Call` | fn | `pluginsdk/plugin.go:548` | 发起 Context Call |
| `Plugin.CallWithCallback` | fn | `pluginsdk/plugin.go:557` | 发起流式调用 |
| `Plugin.CallInContext` | fn | `pluginsdk/plugin.go:562` | 在 Execution Context 中调用 |
| `Plugin.EmitWithIDInContext` | fn | `pluginsdk/plugin.go:604` | 发送归因事件 |

## 4. Agent

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `turn` | fn | `plugins/agent/turn.go:93` | 创建 Turn 并串行化 Session |
| `cancel` | fn | `plugins/agent/turn.go:156` | 取消活动 Turn |
| `appendFacts` | fn | `plugins/agent/turn.go:212` | 写 Session Fact |
| `assembleMessages` | fn | `plugins/agent/turn.go:222` | 读取 Session 投影 |
| `buildModelContext` | fn | `plugins/agent/turn.go:237` | 组装模型输入；Model Context 缺失时回退并在 header 标记 degraded |
| `collectTools` | fn | `plugins/agent/turn.go:275` | 收集可选工具 |
| `callTool` | fn | `plugins/agent/turn.go:329` | 执行工具调用 |
| `runTurn` | fn | `plugins/agent/turn.go:351` | 执行主循环并收口终态 |
| `streamAccumulator.apply` | fn | `plugins/agent/stream.go:64` | 累计 LLM 增量 |
| `plugin.updateStream` | fn | `plugins/agent/stream.go:138` | 更新 Session Stream |

## 5. Session

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `create` | fn | `plugins/session/store.go:300` | 创建 Session；拒绝含路径分隔符的 session_id |
| `append` | fn | `plugins/session/store.go:363` | 写入 Fact 并推送 State |
| `derive` | fn | `plugins/session/store.go:511` | 投影模型上下文 |
| `updateStream` | fn | `plugins/session/ui_state.go:129` | 接收流式快照 |
| `selectSession` | fn | `plugins/session/ui_state.go:249` | 选择 UI Session |
| `closeSession` | fn | `plugins/session/ui_state.go:272` | 关闭一个打开会话 |
| `updateSession` | fn | `plugins/session/ui_state.go:338` | 更新名称；Workspace 绑定只读 |
| `state` | fn | `plugins/session/ui_state.go:403` | 重建 UI State |
| `publishAfterAppend` | fn | `plugins/session/ui_state.go:499` | 推送 Fact 变化 |
| `chatStates` | fn | `plugins/session/ui_state.go:537` | 构建打开会话 Panes；单个失败跳过 |
| `chatState` | fn | `plugins/session/ui_state.go:627` | 投影 ChatState |

## 5.1 Session Workspace

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `workspaceStore.ensure` | fn | `plugins/session/workspace_store.go:155` | 规范化路径并注册或复用 Workspace |
| `workspaceStore.selectID` | fn | `plugins/session/workspace_store.go:235` | 保存当前 Workspace 选择 |
| `workspaceStore.remove` | fn | `plugins/session/workspace_store.go:282` | 移除注册但保留目录和 Session |
| `plugin.resolveWorkspaceBinding` | fn | `plugins/session/workspace_binding.go:9` | 创建 Session 时解析 Workspace 身份 |
| `plugin.backfillWorkspaces` | fn | `plugins/session/workspace_binding.go:34` | 启动时为旧 Session 补齐 Workspace ID |

## 6. LLM

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `complete` | fn | `plugins/llm-openai/openai.go:72` | 选择流式或 JSON 模式 |
| `completeJSON` | fn | `plugins/llm-openai/openai.go:111` | 非流式上游调用 |
| `completeStream` | fn | `plugins/llm-openai/openai.go:152` | 流式上游调用 |
| `emitJSON` | fn | `plugins/llm-openai/openai.go:275` | 发送 `llm.chunk` |

## 7. 浏览器

| 符号 | Kind | 位置 | 角色 |
|---|---|---|---|
| `createMountLoader.load` | fn | `internal/webui/static/loader.mjs:91` | 装载并对齐 Mount；移除已下线 Mount |
| `createLiteAgent.call` | fn | `internal/webui/static/lite-agent.js:96` | 发起 Browser Call |
| `createLiteAgent.pickDirectory` | fn | `internal/webui/static/lite-agent.js:72` | 打开 Host 文件夹选择器 |
| `session-history.addWorkspace` | component | `plugins/session/ui/main.js:663` | 选择文件夹并注册 Workspace |
| `createLiteAgent.onUI` | fn | `internal/webui/static/lite-agent.js:196` | 接收 UI Push |
| `session-chat.send` | component | `plugins/session/ui/main.js:1251` | 发起 Turn |
| `session-chat.cancel` | component | `plugins/session/ui/main.js:1289` | 取消 Turn |
| `renderPluginCenter` | fn | `internal/webui/static/app.js:162` | 渲染插件关系视图 |
| `applyBootstrap` | fn | `internal/webui/static/app.js:206` | 更新插件中心与 Mount |

## 8. 核心边

| From | To | Kind | 证据 |
|---|---|---|---|
| `liteagent-server` | `Host` | owns | `cmd/liteagent-server/server.go:46` |
| `agent` | `session` | depends_on | `plugins/agent/plugin.json:8` |
| `agent` | `llm-openai` | depends_on | `plugins/agent/plugin.json:8` |
| `agent` | `session` | calls | `plugins/agent/turn.go:186` |
| `agent` | `llm-openai` | calls | `plugins/agent/turn.go:473` |
| `agent` | `model-context` | calls | `plugins/agent/turn.go:243` |
| `agent` | `tools` | calls | `plugins/agent/turn.go:280` |
| `llm-openai` | `agent` | emits event | `plugins/llm-openai/main.go:43` |
| `agent` | `session` | stream update | `plugins/agent/stream.go:138` |
| `session` | `WebUI` | UI Push | `plugins/session/ui_state.go:613` |
| `WebUI` | `Host` | Browser Call | `internal/webui/bridge.go:88` |

`agent` 对 `model-context` 和 `tools` 是弱依赖：调用失败时回退 `session.derive`，或跳过工具阶段；两种情况都会写入 `request_header.meta.degraded`，不会静默降级。

## 9. 未核实项

上游 API 的实际鉴权和模型行为由部署配置决定。本文只覆盖 `llm-openai` 的本地请求链路。