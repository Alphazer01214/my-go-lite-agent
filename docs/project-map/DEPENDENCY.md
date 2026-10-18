# 依赖与调用图

本文按 L0、L1 和核心 L3 展示关系。节点可在 [INDEX.md](INDEX.md) 中定位。

## 1. L0 与 L1

```mermaid
flowchart LR
  Server["liteagent-server (entry) ::cmd/liteagent-server/server.go:25"] -->|owns| Host["Host (runtime) ::internal/host/host.go:204"]
  Server -->|owns| WebUI["WebUI Server (runtime) ::internal/webui/server.go:37"]
  Host -->|routes| Agent["agent (plugin) ::plugins/agent/main.go:54"]
  Host -->|routes| Session["session (plugin) ::plugins/session/main.go:56"]
  Host -->|routes| LLM["llm-openai (plugin) ::plugins/llm-openai/main.go:54"]
  Host -->|routes| Tools["tools (plugin) ::plugins/tools/main.go:33"]
  Host -->|routes| Context["model-context (plugin) ::plugins/model-context/main.go:34"]
  Host -->|pipeline| Sandbox["sandbox (plugin) ::plugins/sandbox/main.go:23"]
  Host -->|routes| File["file-tools (plugin) ::plugins/file-tools/main.go:28"]
  Host -->|routes| Shell["shell-tools (plugin) ::plugins/shell-tools/main.go:37"]
  Host -->|routes| Web["web-tools (plugin) ::plugins/web-tools/main.go:17"]
  Host -->|routes| Subagent["subagent (plugin) ::plugins/subagent/main.go:24"]
  Agent -->|depends_on| Session
  Agent -->|depends_on| LLM
  Agent -->|depends_on| Tools
  Agent -->|depends_on| Context
  Context -->|depends_on| Tools
  File -->|depends_on| Tools
  Shell -->|depends_on| Tools
  Web -->|depends_on| Tools
  Sandbox -->|consumes| Tools
  Subagent -->|depends_on| Agent
  Session -->|publishes ui.state| Host
  Agent -->|publishes session stream| Session
```

## 2. Turn 主链

```mermaid
flowchart LR
  UI["session-chat.send (component) ::plugins/session/ui/main.js:827"] -->|loop.turn| Bridge["serveBrowserCall (fn) ::internal/webui/bridge.go:23"]
  Bridge -->|CallWithEvents| Call["CallWithEventsInContext (fn) ::internal/host/call.go:23"]
  Call -->|loop.turn| Turn["turn (method) ::plugins/agent/turn.go:93"]
  Turn -->|session.create| Create["create (method) ::plugins/session/store.go:204"]
  Turn -->|session.append| Append["append (method) ::plugins/session/store.go:247"]
  Turn -->|model-context.build| Build["build (method) ::plugins/model-context/builder.go:52"]
  Build -->|session.derive| Derive["derive (method) ::plugins/session/store.go:379"]
  Build -->|tools.list| List["list (method) ::plugins/tools/main.go:127"]
  Turn -->|tools.call| ToolCall["call (method) ::plugins/tools/main.go:136"]
  ToolCall -->|target method| Provider["Tool Provider"]
  Turn -->|llm.complete| Complete["complete (method) ::plugins/llm-openai/openai.go:71"]
  Complete -->|llm.chunk| Accumulate["streamAccumulator.apply (method) ::plugins/agent/stream.go:81"]
  Accumulate -->|session.stream.update| Update["updateStream (method) ::plugins/session/ui_state.go:86"]
  Update -->|ui.command| Publish["PublishCommand (method) ::internal/webui/publisher.go:176"]
  Publish -->|UI Push| UI
  Append -->|ui.state| Chat["chatState (method) ::plugins/session/ui_state.go:295"]
  Chat -->|State Push| UI
```

## 3. Frame 路由

```mermaid
flowchart LR
  SDK["Plugin.Listen (method) ::pluginsdk/plugin.go:143"] -->|req| Dispatch["Plugin.dispatch (method) ::pluginsdk/plugin.go:198"]
  SDK -->|call| Pending["Plugin.callPending (method) ::pluginsdk/plugin.go:299"]
  Pending -->|Frame req| HostRead["Host.read (method) ::internal/host/transport.go:395"]
  HostRead --> Route["Host.routeFrame (method) ::internal/host/transport.go:19"]
  Route --> Handle["Host.handleRequest (method) ::internal/host/transport.go:50"]
  Handle -->|forward| Forward["Host.forward (method) ::internal/host/transport.go:307"]
  Forward --> Target["Plugin.dispatch (method) ::pluginsdk/plugin.go:198"]
  Target -->|res| Response["Host.handleResponse (method) ::internal/host/transport.go:206"]
  Response --> Wait["Plugin.waitResult (method) ::pluginsdk/plugin.go:267"]
  Target -->|attributed evt| Event["Host.handleEvent (method) ::internal/host/transport.go:239"]
  Event --> Wait
```

## 4. 启动关系

```mermaid
flowchart BT
  Tools["tools (plugin) ::plugins/tools/main.go:33"] --> Context["model-context (plugin) ::plugins/model-context/main.go:34"]
  Tools --> File["file-tools (plugin) ::plugins/file-tools/main.go:28"]
  Tools --> Shell["shell-tools (plugin) ::plugins/shell-tools/main.go:37"]
  Tools --> Web["web-tools (plugin) ::plugins/web-tools/main.go:17"]
  Tools --> Sandbox["sandbox (plugin) ::plugins/sandbox/main.go:23"]
  Context --> Agent["agent (plugin) ::plugins/agent/main.go:54"]
  Session["session (plugin) ::plugins/session/main.go:56"] --> Agent["agent (plugin) ::plugins/agent/main.go:54"]
  LLM["llm-openai (plugin) ::plugins/llm-openai/main.go:54"] --> Agent
  Agent --> Subagent["subagent (plugin) ::plugins/subagent/main.go:24"]
  Scan["AutostartClosure (fn) ::internal/plugin/mount.go:8"] -->|DFS post-order| Session
  Scan -->|DFS post-order| Tools
  Scan -->|DFS post-order| LLM
  Scan -->|then| Agent
```

箭头由依赖方指向被依赖插件。启动图使用相同方向。

## 5. 取消与异常

```mermaid
flowchart LR
  CancelUI["session-chat.cancel (component) ::plugins/session/ui/main.js:848"] -->|loop.cancel| Cancel["cancel (method) ::plugins/agent/turn.go:139"]
  Cancel --> State["turnState.markCancelled (method) ::plugins/agent/turn.go:33"]
  State --> Run["runTurn (method) ::plugins/agent/turn.go:259"]
  Run -->|turn_end(cancelled)| Append["session.append ::plugins/session/store.go:247"]
  Run -->|turn_end(error)| Append
  Append --> StatePush["chatState ::plugins/session/ui_state.go:295"]
  StatePush -->|State Push| CancelUI
```

## 6. 边说明

| 边 | 含义 | 证据 |
|---|---|---|
| Host → Plugin | 装载、路由、pending | `internal/host/` |
| Agent → Session | `depends_on` 与调用 | `plugins/agent/plugin.json` |
| Agent → LLM | `depends_on` 与调用 | `plugins/agent/plugin.json` |
| Agent → Tools | `depends_on`、`tools.list/call` | `plugins/agent/plugin.json` |
| Agent → Model Context | `depends_on`、`model-context.build` | `plugins/agent/plugin.json` |
| Tool Provider → Tools | 启动依赖与 `tools.register` | `plugins/*-tools/plugin.json` |
| Sandbox → Tool Call | Pipeline Consumer | `plugins/sandbox/plugin.json` |
| Subagent → Agent | `depends_on`、`agent.loop.turn` | `plugins/subagent/plugin.json` |
| LLM → Agent | `llm.chunk` 归因事件 | `plugins/llm-openai/main.go:60` |
| Session → WebUI | UI State、Command | `internal/webui/publisher.go:147` |
| WebUI → Host | Browser Call | `internal/webui/bridge.go:23` |
