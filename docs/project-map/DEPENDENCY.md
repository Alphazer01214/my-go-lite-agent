# 依赖与调用图

本文按 L0、L1 和核心 L3 展示关系。节点可在 [INDEX.md](INDEX.md) 中定位。

## 1. L0 与 L1

```mermaid
flowchart LR
  Server["liteagent-server (entry) ::cmd/liteagent-server/server.go:27"] -->|owns| Host["Host (runtime) ::internal/host/host.go:17"]
  Server -->|owns| WebUI["WebUI Server (runtime) ::internal/webui/server.go:38"]
  Host -->|routes| Agent["agent (plugin) ::plugins/agent/main.go:54"]
  Host -->|routes| Session["session (plugin) ::plugins/session/main.go:59"]
  Host -->|routes| LLM["llm-openai (plugin) ::plugins/llm-openai/main.go:37"]
  Agent -->|depends_on| Session
  Agent -->|depends_on| LLM
  Agent -.->|requires| ToolsCap["tools capability（当前未装载）"]
  Agent -.->|requires| ContextCap["model-context capability（当前未装载）"]
  Session -->|publishes ui.state| Host
  Agent -->|publishes session stream| Session
```

## 2. Turn 主链

```mermaid
flowchart LR
  UI["session-chat.send (component) ::plugins/session/ui/main.js:1251"] -->|loop.turn| Bridge["serveBrowserCall (fn) ::internal/webui/bridge.go:23"]
  Bridge -->|CallWithEvents| Call["CallWithEventsInContext (fn) ::internal/host/call.go:25"]
  Call -->|loop.turn| Turn["turn (method) ::plugins/agent/turn.go:93"]
  Turn -->|session.create| Create["create (method) ::plugins/session/store.go:300"]
  Turn -->|session.append| Append["append (method) ::plugins/session/store.go:363"]
  Turn -->|buildModelContext| Build["buildModelContext (fn) ::plugins/agent/turn.go:237"]
  Build -.->|model-context.build（缺失时回退）| Fallback["session.derive (method) ::plugins/session/store.go:511"]
  Build -.->|tools.list（可缺省）| ToolsX["tools capability（当前未装载）"]
  Turn -.->|tools.call（可缺省）| ToolsX
  Turn -->|llm.complete| Complete["complete (method) ::plugins/llm-openai/openai.go:72"]
  Complete -->|llm.chunk| Accumulate["streamAccumulator.apply (method) ::plugins/agent/stream.go:64"]
  Accumulate -->|session.stream.update| Update["updateStream (method) ::plugins/session/ui_state.go:129"]
  Update -->|ui.command| Publish["PublishCommand (method) ::internal/webui/publisher.go:187"]
  Publish -->|UI Push| UI
  Append -->|ui.state| Chat["chatState (method) ::plugins/session/ui_state.go:627"]
  Chat -->|State Push| UI
```

`Build` 的两条虚线是弱依赖：两者不可用时 Turn 仍然继续，但会把原因写进 `request_header.meta.degraded`。

## 3. Frame 路由

```mermaid
flowchart LR
  SDK["Plugin.Listen (method) ::pluginsdk/plugin.go:225"] -->|req| Dispatch["Plugin.dispatch (method) ::pluginsdk/plugin.go:295"]
  SDK -->|call| Pending["Plugin.callPendingInContext (method) ::pluginsdk/plugin.go:503"]
  Pending -->|Frame req| HostRead["Host.read (method) ::internal/host/transport.go:507"]
  HostRead --> Route["Host.routeFrame (method) ::internal/host/transport.go:21"]
  Route --> Handle["Host.handleRequest (method) ::internal/host/transport.go:54"]
  Handle -->|forward| Forward["Host.forwardPipeline (method) ::internal/host/transport.go:419"]
  Forward --> Target["Plugin.dispatch (method) ::pluginsdk/plugin.go:295"]
  Target -->|res| Response["Host.handleResponse (method) ::internal/host/transport.go:318"]
  Response --> Wait["Plugin.waitResult (method) ::pluginsdk/plugin.go:450"]
  Target -->|attributed evt| Event["Host.handleEvent (method) ::internal/host/transport.go:350"]
  Event --> Wait
```

## 4. 启动关系

```mermaid
flowchart BT
  Session["session (plugin) ::plugins/session/main.go:59"] --> Agent["agent (plugin) ::plugins/agent/main.go:54"]
  LLM["llm-openai (plugin) ::plugins/llm-openai/main.go:37"] --> Agent
  Scan["AutostartClosure (fn) ::internal/plugin/mount.go:8"] -->|DFS post-order| Session
  Scan -->|DFS post-order| LLM
  Scan -->|then| Agent
```

箭头由依赖方指向被依赖插件。启动图使用相同方向。

入口先构造 Host、接好 UI Publisher，再调用 `Host.Start` 挂载插件
（`cmd/liteagent-server/server.go:46` → `:53` → `:55`）。顺序反了会让插件启动期的 `ui.state` 拿到 `ui_not_mounted`。

## 5. 取消与异常

```mermaid
flowchart LR
  CancelUI["session-chat.cancel (component) ::plugins/session/ui/main.js:1289"] -->|loop.cancel| Cancel["cancel (method) ::plugins/agent/turn.go:156"]
  Cancel --> State["turnState.markCancelled (method) ::plugins/agent/turn.go:33"]
  State --> Run["runTurn (method) ::plugins/agent/turn.go:351"]
  Run -->|turn_end(cancelled)| Append["session.append ::plugins/session/store.go:363"]
  Run -->|turn_end(error)| Append
  Append --> StatePush["chatState ::plugins/session/ui_state.go:627"]
  StatePush -->|State Push| CancelUI
```

## 6. 边说明

| 边 | 含义 | 证据 |
|---|---|---|
| Host → Plugin | 装载、路由、pending | `internal/host/` |
| Agent → Session | `depends_on` 与调用 | `plugins/agent/plugin.json` |
| Agent → LLM | `depends_on` 与调用 | `plugins/agent/plugin.json` |
| Agent → Model Context | 弱依赖 `model-context.build`，缺失时回退并标记 degraded | `plugins/agent/turn.go:243` |
| Agent → Tools | 弱依赖 `tools.list/call`，缺失时跳过并标记 degraded | `plugins/agent/turn.go:280` |
| LLM → Agent | `llm.chunk` 归因事件 | `plugins/llm-openai/main.go:43` |
| Session → WebUI | UI State、Command | `internal/webui/publisher.go:155` |
| WebUI → Host | Browser Call | `internal/webui/bridge.go:23` |