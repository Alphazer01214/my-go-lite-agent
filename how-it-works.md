# my-go-lite-agent：一切皆插件，是如何实现的

> 本文以**一次真实的 Agent 行动**（用户输入 `please echo me`）为案例，逐帧、逐代码地讲清
> "一切皆插件" 的定义、实现机制与运行时数据流。

---

## 目录

1. [总览：Host 是薄内核](#1-总览host-是薄内核)
2. [四层装配：从磁盘到运行时](#2-四层装配从磁盘到运行时)
3. [线协议：Frame](#3-线协议frame)
4. [星型路由：插件之间永不直连](#4-星型路由插件之间永不直连)
5. [案例：一次 Agent 行动的完整时间线](#5-案例一次-agent-行动的完整时间线)
6. [横切能力：Waterfall / 崩溃自愈 / 超时](#6-横切能力waterfall--崩溃自愈--超时)
7. [连 UI 和 Agent Loop 也是插件](#7-连-ui-和-agent-loop-也是插件)
8. [代码地图与常量速查](#8-代码地图与常量速查)

---

## 1. 总览：Host 是薄内核

`cmd/host/main.go` 第一行注释就写着：

```go
// Command host is the thin kernel entry.
package main
```

Host 进程本身**不实现任何业务能力**，只做三件事：

1. **发现**磁盘上的插件（`discovery`）
2. **挂载**用户选中的插件为子进程（`serve`）
3. 在插件之间做**星型路由**（star topology）

所有真正干活的东西都是独立可执行文件，且都用 Capability 名注册。
这些 Capability 名集中定义在 `serve/serve.go`：

```go
const SessionCap      = "session"        // 会话日志
const AgentCap        = "agent"          // Host 自有的 agent/request、agent.inject
const LLMCap          = "llm"            // 模型调用
const SystemPromptCap = "system-prompt"  // 上下文管理器（ADR-0006）
const LoopCap         = "loop"           // 整个 Agent Loop（ADR-0003），可被替换
const ToolsCap        = "tools"          // 工具
const PresentationCap = "presentation"   // UI 渲染意图（广播 evt）
// 另见 serve/waterfall.go:
const InterceptorCap  = "interceptor"    // 拦截器
```

**"一切皆插件"最硬核的证据**：连推理主循环都是可替换 Capability。
`serve/serve.go` 的 `runTurn` 开头：

```go
s.mu.Lock()
loopOwner, hasExternalLoop := s.provides[LoopCap]
s.mu.Unlock()
if hasExternalLoop {
    return s.runExternalTurn(loopOwner, sessionID, userInput) // 用外部插件跑整轮
}
// 否则走 Host 内编译进去的默认 Loop
```

默认 Loop 只是"没人提供 `loop` 能力时的兜底实现"。挂一个提供 `loop` 的插件，整个推理循环被换掉。

---

## 2. 四层装配：从磁盘到运行时

```
              plugin.json                discovery.Scan        assembly.Resolve       serve.Start
磁盘目录结构  ───────────────►  发现[]Found  ────────►  交集出 Plan  ────────►  启动子进程 + 建路由表
```

### 第 1 层：Manifest —— 插件的身份声明（`plugin/manifest.go`）

每个插件目录下有一个 `plugin.json`（测试 fixture 见 `cmd/host/lifecycle_fixtures_test.go`）：

```json
{
  "name": "session",
  "version": "0.1.0",
  "protocol": 2,
  "provides": ["session"],
  "consumes": [],
  "entry": "session.exe",
  "timeoutMs": 300
}
```

```go
type Manifest struct {
    Name      string   `json:"name"`
    Version   string   `json:"version"`
    Protocol  int      `json:"protocol"`
    Provides  []string `json:"provides"`   // 对外提供哪些 Capability
    Consumes  []string `json:"consumes"`   // 依赖别人提供哪些 Capability
    Entry     string   `json:"entry"`      // 可执行文件名
    TimeoutMs int      `json:"timeoutMs,omitempty"`
}
```

`Validate()`（`manifest.go:43`）强制校验 name/version/protocol/entry 非空，且：

```go
if m.Protocol != CurrentProtocol { // CurrentProtocol = 1
    return fmt.Errorf("protocol must be %d, got %d", CurrentProtocol, m.Protocol)
}
```

`Provides/Consumes` 是"能力图"的边，必须非空串。

### 第 2 层：Discovery —— 只观察，不启动（`discovery/discovery.go`）

```go
// Scan looks at <root>/<child>/plugin.json one level deep. It never launches Plugins.
func Scan(root string) Result
```

扫一层子目录，读 `plugin.json`、校验、检查 entry 存在，收集为 `[]Found` 并**按 name 排序**。
关键设计：发现阶段**绝不起进程**，无副作用。

```go
type Found struct {
    Dir      string
    Manifest plugin.Manifest
}
type Result struct {
    Plugins []Found
    Errors  []ScanError
}
```

### 第 3 层：Assembly —— 用户决定挂载谁（`assembly/assembly.go`）

用户配置：

```go
type Config struct {
    Plugins []string `json:"plugins"`
}
```

`Resolve()` 把配置和 discovery 结果做**交集**：

```go
type Plan struct {
    Mounted   []discovery.Found  // 被选中的
    Unmounted []discovery.Found  // 被发现但没挂
    Missing   []string           // 配置里写了但磁盘上没有
}
```

**"一切皆插件"的关键一环在组装阶段**：同一份代码 + 不同 `plugins` 列表 = 完全不同的 agent。
只挂 `session + llm` 就是纯聊天；加 `filetools` 才是编码 agent；加 `interceptor` 才有策略拦截。
能力是**组合出来的**，不是编译进去的。

### 第 4 层：Serve —— 运行时内核（`serve/serve.go`）

`serve.Start(mounted)`（`serve.go:125`）依次做四件事：

```go
// 1. 先建能力注册表，检测能力冲突（"一能力一提供者"）
for _, p := range mounted {
    for _, capName := range p.Manifest.Provides {
        if owner, ok := s.provides[capName]; ok {
            return nil, fmt.Errorf("capability %q provided by both %s and %s", capName, owner, p.Manifest.Name)
        }
        s.provides[capName] = p.Manifest.Name
    }
}
// 2. 依次 launch 每个插件的子进程
// 3. checkConsumes：校验依赖图闭合
// 4. 启动 readLoop 读每个插件的 stdout
```

`provides map[string]string` 就是**能力 → 插件名**的路由表。`checkConsumes` 保证每个 `consumes`
都能在 `provides` 里找到，否则拒绝启动——依赖在**启动时静态校验**。

---

## 3. 线协议：Frame

Host 与插件之间走 **stdio**，消息格式是 `uint32 大端长度 + JSON body`（`protocol/frame.go`）：

```go
type Frame struct {
    V       int             `json:"v"`       // 协议版本 = 1
    ID      string          `json:"id"`      // 请求/响应关联 id
    Type    string          `json:"type"`    // req | res | evt
    Cap     string          `json:"cap"`     // 目标 Capability
    Method  string          `json:"method"`  // 方法
    Payload json.RawMessage `json:"payload,omitempty"`
    Error   *FrameError     `json:"error,omitempty"`
}
```

三种帧类型：

| Type | 含义 | 期望响应 |
|------|------|---------|
| `req` | 请求 | 是（对应 `res`） |
| `res` | 响应 | — |
| `evt` | 事件 | 否（如流式 chunk、Presentation 卡、状态） |

编码/解码（`frame.go`）：

```go
func WriteFrame(w io.Writer, f *Frame) error {
    body, _ := json.Marshal(f)
    var hdr [4]byte
    binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
    w.Write(hdr[:]); w.Write(body)
}
func ReadFrame(r io.Reader) (*Frame, error) {
    io.ReadFull(r, hdr[:])
    n := binary.BigEndian.Uint32(hdr[:])
    if n == 0 || n > maxFrameSize { /* maxFrameSize = 16 << 20 */ }
    ...
}
```

---

## 4. 星型路由：插件之间永不直连

这是"一切皆插件"能成立的基础。插件 A 要调插件 B，**不能直接连 B**，必须发 `req` 给 Host，
Host 查路由表转发（`serve.go` 的 `routeRequest`）：

```go
owner, ok := s.provides[f.Cap]
if !ok || owner == from {                 // 能力不存在，或想调自己 → 拒绝
    ... Error{Code: "capability_unavailable"} ...
    return
}
to := DefaultCallTimeout
if p := s.plugins[owner]; p != nil { to = p.timeout }
s.seq++
fwdID := fmt.Sprintf("fwd-%d", s.seq)      // 替换 id，避免与 caller 的 id 冲突
s.pending[fwdID] = &wait{kind: waitPlugin, caller: from, target: owner, origID: f.ID}
...
fwd := *f; fwd.ID = fwdID
s.writeTo(owner, &fwd)                     // 转发
```

B 回 `res` 时，`complete()` 把 id 换回 `origID`，再回给 A：

```go
out := *f
out.ID = w.origID
s.writeTo(w.caller, &out)
```

**为什么必须星型？** 因为只有 Host 在路径上，才能统一插入：超时、拦截器（Waterfall）、
审计、崩溃恢复。如果插件直连，这些横切能力就无处安放。这是"一切皆插件"的代价与红利。

### 插件作者只写业务：pluginsdk（`pluginsdk/server.go`）

插件作者不碰 Frame 循环，用 SDK。`echo` 插件全部代码：

```go
func main() {
    s := pluginsdk.New()
    s.Handle("echo", "echo", func(req *pluginsdk.Request) (json.RawMessage, error) {
        return req.Payload, nil       // 请求原样当响应
    })
    _ = s.Serve()
}
```

SDK 提供四个原语：

- `Handle(cap, method, handler)` —— 注册能力方法（`handlers[cap+"."+method] = h`）
- `Call(cap, method, payload)` —— 通过 Host 调别的能力（星型），阻塞等 `res`
- `Emit()` / `EmitTo(id,...)` —— 发事件；`EmitTo` 带 id 以便 Host 归属到某次 Call
- `Serve()` —— 读 stdin 循环，`req` 开 goroutine 分发，`res` 匹配 pending channel

---

## 5. 案例：一次 Agent 行动的完整时间线

### 5.1 场景与装配

命令（对应 `cmd/host/tool_test.go: TestToolCallPathOneTurn`）：

```
host -plugins <dir> -assembly assembly.json -turn "please echo me" -session-derive
```

装配文件：

```json
{"plugins":["session","fakellm","echotool"]}
```

参与者（各为独立子进程）：

| 插件 | 提供 | 作用 |
|------|------|------|
| `session` | `session` | append-only 会话日志 + derive |
| `fakellm` | `llm` | 确定性假 LLM（首跳请求工具调用，次跳复述工具结果） |
| `echotool` | `tools` | 提供 `echo_text` 工具，并广播 Presentation 卡 |

`serve.Start` 后路由表：

```
provides = { "session" → "session", "llm" → "fakellm", "tools" → "echotool" }
```

### 5.2 Host 侧驱动：Default Loop（`serve.go` 的 `runTurn`）

`runTurn` 结构（简化，标 ⭐ 为下面会逐帧展开的调用）：

```
append(turn_start)                        ⭐ S1
emitStatus("running")                     → 渲染介质打印 "Thinking…"
system-prompt.assemble                    （本装配无 contextmanager → 返回 ""）
append(user message)                       ⭐ S2
tools.list                                ⭐ S3  （收集工具 schema）
append(run_subagent schema 到内存 schemas)（因挂了 tools 且 allowSubagent）
loop step = 0：
    append(step_start)                     ⭐ S4
    agent/request (DeriveMessages 校验不变量) ⭐ S5
    append(request_header)                 ⭐ S6
    llm.complete                           ⭐ S7
    → 有 tool_calls → append(tool_call)    ⭐ S8
       → tools.call (echo_text)            ⭐ S9
       → append(tool_result)               ⭐ S10
    append(step_end reason=tools)          ⭐ S11
loop step = 1：
    append(step_start) … llm.complete → 无 tool_calls → append(assistant)  ⭐ S12
    append(step_end reason=completed)      ⭐ S13
    break
DeriveMessages() → 最终 Model Context
deferred: append(turn_end) + emitStatus("idle")  ⭐ S14
```

### 5.3 逐帧展开

下面每一"帧"都是 Host 与插件之间的一次 `req`/`res`。约定：
Host→插件为 `→`，插件→Host 为 `←`。

---

**⭐ S1 — 记录 turn 开始**

Host 调 `AppendSessionFacts`（`serve.go:740`）：

```
→ session  req  {v:1, id:"host-1", cap:"session", method:"append",
                 payload:{"type":"turn_start","role":"host","meta":{"turn":1}}}
← session  res  {v:1, id:"host-1", cap:"session", method:"append",
                 payload:{"seq":1}}
```

Session 插件把它作为第 1 条 fact 追加（`plugins/session/main.go` 的 append handler）。
注意 `AppendSessionFacts` 用 `Call`（Host 发起），id 形如 `host-1`。

---

**⭐ S2 — 记录用户消息**

```
→ session  req  {id:"host-2", cap:"session", method:"append",
                 payload:{"type":"message","role":"user","content":"please echo me"}}
← session  res  {id:"host-2", payload:{"seq":2}}
```

此刻 Session Log：`[turn_start, user]`。

---

**⭐ S3 — 收集工具 schema**

Host 调 `CollectToolSchemas`（`serve.go:1163`）：

```
→ echotool req  {id:"host-3", cap:"tools", method:"list", payload:{}}
← echotool res  {id:"host-3", cap:"tools",
                 payload:{"tools":[{"name":"echo_text",
                          "description":"Echo text back to the caller",
                          "input_schema":{...}}]}}
```

Host 接着（`serve.go:953`）往内存 `schemas` 追加 `run_subagent` 的 schema。
注意：`run_subagent` 只注入到发给 LLM 的 schema 里，**不落 Session Log**。

---

**⭐ S4 — 记录 step 开始**

```
→ session  req  {id:"host-4", method:"append",
                 payload:{"type":"step_start","role":"host","meta":{"turn":1,"step":1}}}
← session  res  {id:"host-4", payload:{"seq":3}}
```

---

**⭐ S5 — agent/request：校验 log 不变量（ADR-0002）**

Host 内部调 `AgentRequest(sessionID, nil)`，进而 `DeriveMessages`：

```
→ session  req  {id:"host-5", cap:"session", method:"derive", payload:{}}
← session  res  {id:"host-5", payload:{"messages":[
                 {"role":"user","content":"please echo me"}
               ],"count":1}}
```

`derive()` 的核心（`plugins/session/main.go:156`）——**纯投影**：
只把 `type=message / tool_call / tool_result` 的 fact 还原成 chat message：

```go
switch f.Type {
case "message":
    msgs = append(msgs, Message{Role: f.Role, Content: f.Content})
case "tool_call":
    ... 还原 ToolCalls ...
case "tool_result":
    ... 还原 ToolCallID ...
}
```

`AgentRequest`（`serve.go:786`）：claimed 为空 → `Rebuilt=true`；非空则必须与 derive 逐字段相等，
否则返回 `session_invariant_violation`。这就是"模型上下文必须可从日志重建"的强制约束。

---

**⭐ S6 — 记录 Request Header**

```
→ session  req  {id:"host-6", method:"append",
                 payload:{"type":"request_header","role":"host",
                          "meta":{"provider":"default","model":"default","step":1,"turn":1}}}
← session  res  {id:"host-6", payload:{"seq":4}}
```

---

**⭐ S7 — llm.complete（第一跳）**

`CallStreamOn`（`serve.go:485`）发起，并挂一个实时回调处理 `evt`：

```
→ fakellm req  {id:"host-7", cap:"llm", method:"complete",
                payload:{"messages":[{"role":"user","content":"please echo me"}],
                         "tools":[{"name":"echo_text",...},{"name":"run_subagent",...}]}}
← fakellm res  {id:"host-7", cap:"llm",
                payload:{"content":"","tool_calls":[
                          {"id":"call-1","name":"echo_text",
                           "arguments":{"text":"please echo me"}}]}}
```

`fakellm` 的策略（`plugins/fakellm/main.go:57`）：

```go
// First model hop: tools available and none used yet → request one tool call.
if len(in.Tools) > 0 && !hasToolMsg {
    args, _ := json.Marshal(map[string]string{"text": lastUser})
    out, _ := json.Marshal(map[string]any{
        "content": "",
        "tool_calls": []toolCall{{ID:"call-1", Name: in.Tools[0].Name, Arguments: args}},
    })
    return out, nil
}
```

> 真实的 `llm-openai` 插件在此处会发 `presentation.stream` 的 `start/chunk/end`
> 事件流（SSE），Host 的 `extractStreamDelta` 把 chunk 透传给渲染介质。

由于本跳 `content=""`，没有流式文本。

---

**⭐ S8 — 记录 tool_call**

Host 把 LLM 返回的工具调用作为一个 fact 落盘（`serve.go:1096`）：

```
→ session  req  {id:"host-8", method:"append",
                 payload:{"type":"tool_call","role":"assistant","content":"",
                          "meta":{"tool_calls":[{"id":"call-1","tool_call_id":"call-1",
                                   "name":"echo_text","arguments":{"text":"please echo me"}}]}}}
← session  res  {id:"host-8", payload:{"seq":5}}
```

落盘前，Host 会**规范化 tool_call id**（`serve.go:1074`）：空或重复的 id 改成 `call_<step>_<n>`，
以规避 OpenAI/DeepSeek 对 `tool_call_id` 的校验。

---

**⭐ S9 — tools.call（执行工具）**

`CallTool`（`serve.go:1263`）先触发 `OnToolCall`（渲染介质打印 `⏺ echo_text`），再走星型调用：

```
→ echotool req  {id:"host-9", cap:"tools", method:"call",
                 payload:{"name":"echo_text","arguments":{"text":"please echo me"}}}
```

echotool 内部（`plugins/echotool/main.go:47`）：
1. 校验 name，解析 arguments
2. 广播一张纯投影的 Presentation 卡：

```
← echotool evt  {v:1, type:"evt", cap:"presentation", method:"card",   // 注意：无 id（广播）
                 payload:{"cardType":"echo_result","tool":"echo_text",
                          "data":{"input":"please echo me","output":"please echo me"}}}
```

Host 的 `collectEvent`（`serve.go:295`）**在 id 过滤之前**先记录这张卡：

```go
if f.Cap == PresentationCap && f.Method == PresentationCardMethod {
    s.recordCard(f)
}
if f.ID == "" { return }   // 广播帧到此为止，不归属任何 Call
```

3. 返回工具结果：

```
← echotool res  {id:"host-9", cap:"tools", payload:{"content":"please echo me"}}
```

Host 把它转发回调用者（这里是 Host 自己），并在渲染介质上以 expandable 展示
（`serve.go:1117` 直接调 `s.OnRender("expandable", tc.Name, ...)`）。

---

**⭐ S10 — 记录 tool_result**

```
→ session  req  {id:"host-10", method:"append",
                 payload:{"type":"tool_result","role":"tool","content":"please echo me",
                          "meta":{"tool_call_id":"call-1"}}}
← session  res  {id:"host-10", payload:{"seq":6}}
```

若工具返回了 `additionalContexts`，Host 会在 tool_result **之后**逐条追加为 `message`
fact（`serve.go:1128`，US16）。

---

**⭐ S11 — 记录 step 结束（reason=tools）**

```
→ session  req  {id:"host-11", method:"append",
                 payload:{"type":"step_end","role":"host",
                          "meta":{"turn":1,"step":1,"reason":"tools"}}}
← session  res  {id:"host-11", payload:{"seq":7}}
```

此刻 Session Log：

```
1 turn_start
2 message(user)   "please echo me"
3 step_start      step=1
4 request_header  step=1
5 tool_call(assistant) [echo_text(call-1)]
6 tool_result(tool)     "please echo me"
7 step_end        reason=tools
```

---

**⭐ S12 — 第二跳 llm.complete → 最终回复**

step=1：
- append `step_start` → seq 8
- `agent/request` → derive 现在返回 3 条 message：

```
[ {role:user, content:"please echo me"},
  {role:assistant, tool_calls:[{id:"call-1",name:"echo_text",...}]},
  {role:tool, content:"please echo me", tool_call_id:"call-1"} ]
```

- append `request_header` → seq 9
- `llm.complete`：

```
→ fakellm req  {id:"host-12", cap:"llm", method:"complete",
                payload:{"messages":[ user, assistant(tool_calls), tool ], "tools":[...]}}
```

fakellm 这次 `hasToolMsg == true`，走最终回复分支，并**流式发 chunk 事件**
（`plugins/fakellm/main.go:85`）：

```
← fakellm evt  {type:"evt", id:"host-12", cap:"llm", method:"chunk", payload:{"delta":"Tool "}}
← fakellm evt  {type:"evt", id:"host-12", cap:"llm", method:"chunk", payload:{"delta":"said: "}}
← fakellm evt  {type:"evt", id:"host-12", cap:"llm", method:"chunk", payload:{"delta":"please echo me"}}
← fakellm res  {id:"host-12", cap:"llm", payload:{"content":"Tool said: please echo me"}}
```

这些 `id=host-12` 的 `evt` 会被 `collectEvent` 归属到等待中的 Host Call，
回调 `extractStreamDelta` 取出 `delta` 调 `OnStreamDelta` → 渲染介质实时打印。

因为本跳 `tool_calls` 为空，走最终回复分支（`serve.go:1044`）：

```
→ session  req  {id:"host-13", method:"append",
                 payload:{"type":"message","role":"assistant","content":"Tool said: please echo me"}}
← session  res  {id:"host-13", payload:{"seq":10}}
```

---

**⭐ S13 — step_end（reason=completed）**

```
→ session  req  {id:"host-14", method:"append",
                 payload:{"type":"step_end","role":"host",
                          "meta":{"turn":1,"step":2,"reason":"completed"}}}
← session  res  {id:"host-14", payload:{"seq":11}}
```

Loop `break`。

---

**⭐ S14 — turn 收尾**

- `DeriveMessages` → 得到最终 Model Context（4 条）：

```
[ user:"please echo me",
  assistant:tool_calls[echo_text],
  tool:"please echo me",
  assistant:"Tool said: please echo me" ]
```

- deferred 收尾（`serve.go:892`）：

```
→ session  req  {id:"host-15", method:"append",
                 payload:{"type":"turn_end","role":"host","meta":{"turn":1,"reason":"completed"}}}
← session  res  {id:"host-15", payload:{"seq":12}}
```
- `emitStatus("idle")`。

CLI 最后打印（`cmd/host/main.go:592`）：

```
turn ok user=please echo me assistant="Tool said: please echo me" chunks=3 tools=[echo_text]
```

这正是 `tool_test.go` 断言的内容：
`user → tool_call → tool result → final assistant` 顺序（`tool_test.go:69`）。

### 5.4 一张图看清"一切皆插件"

```
                 ┌───────────────────────── cmd/host (薄内核, 无状态) ─────────────────────────┐
                 │  Default Loop (RunTurn)                                                   │
                 │   ├─提供 loop 的插件存在？ → runExternalTurn(loopOwner)  ← 可被完全替换     │
                 │   └─否则本地 Loop：编排下方所有调用                                        │
                 │                                                                            │
                 │  provides 路由表: {session→session, llm→fakellm, tools→echotool}           │
                 └───┬───────────────┬────────────────┬───────────────────┬──────────────────┘
             req/res │        req/res│         req/res│            evt(广播)│
                     ▼               ▼                ▼                     ▼
                 ┌────────┐     ┌─────────┐      ┌──────────┐        (Presentation)
                 │ session│     │ fakellm │      │ echotool │         card/render/stream/status
                 │ 子进程 │     │ 子进程  │      │ 子进程   │
                 └────────┘     └─────────┘      └──────────┘
```

- Host 不持有会话状态：Session Log 全在 `session` 进程的内存里
  （`plugins/session/main.go`：`Storage lives only in this process; swap the Plugin to replace the backend.`）
- Host 不懂模型协议：HTTP/SSE 全在 `llm-openai` 进程里
- Host 不懂文件系统：读/写/编辑/搜索全在 `filetools` 进程里
- Host 只懂：**Frame + 路由表 + 编排顺序**

---

## 6. 横切能力：Waterfall / 崩溃自愈 / 超时

这三点让"可插拔"真正可用。它们**全部寄生在星型路由的转发路径上**。

### 6.1 Waterfall：双链拦截（`serve/waterfall.go`）

每次 `routeRequest` 之前，`runWaterfall` 先跑一次：

```go
func (s *Server) runWaterfall(from string, f *protocol.Frame) WaterfallDecision {
    dec := s.callInterceptor(from, f)            // ① 外部 Interceptor 插件（可插拔）
    s.recordAudit(from, f.Cap, f.Method, dec.Action, dec.Reason) // ② 内置审计（强制）
    return dec
}
```

Interceptor 通过 `interceptor.before` 返回 `allow | rewrite | reject`：

| Action | 效果 |
|--------|------|
| `allow` | 原样转发 |
| `rewrite` | 用返回的 `payload` 替换（**空 payload 忽略**，防误清空，`waterfall.go:135`） |
| `reject` | 短路，回收 `interceptor_rejected` |

关键设计（对应 `waterfall_test.go`）：

- **内置审计链不可被插件移除**（安全底线）——`TestWaterfallMandatoryChainSurvivesInterceptorCrash`
- **拦截器崩溃 fail-open**：`interceptor_down → ActionAllow`，一个策略插件挂了不瘫痪整机
- **对 `agent/request` 同样生效**——`routeRequest` 里 Waterfall 在 agent 分支**之前**执行
  （`serve.go:337` 起手就是 `dec := s.runWaterfall(from, f)`），
  见 `TestWaterfallCoversAgentRequest`

### 6.2 崩溃自愈：按需重启（`serve.go:254`）

插件进程挂了 → `readLoop` 读到 EOF → `markUnhealthy`，把所有指向它的 pending
以 `plugin_down` 失败掉。下次调用时 `ensureAlive` 杀旧进程并重新 `launch`。
`CallStreamOn` 遇到 `plugin_down` 会自动**重试一次**：

```go
if res.Frame.Error != nil && res.Frame.Error.Code == "plugin_down" && attempt == 0 {
    lastErr = ...; continue   // 重试
}
```

### 6.3 超时隔离

每个插件有自己的 `timeoutMs`，跨插件调用用 `time.AfterFunc` 兜底：

```go
timer := time.AfterFunc(to, func() {
    ... Error{Code:"timeout", Message: fmt.Sprintf("call to %s timed out after %s", owner, to)} ...
})
```

慢插件（`plugins/slow`，`timeoutMs: 300`）不会拖死别的插件。

---

## 7. 连 UI 和 Agent Loop 也是插件

### 7.1 Presentation：UI 渲染也是插件（`pluginsdk/presentation.go`）

插件不知道终端长什么样，只发**分类渲染意图**事件：

```go
s.EmitMarkdown(text)                 // kind=markdown
s.EmitExpandable(title, body, ...)   // kind=expandable
s.EmitMessage("error", "...")        // kind=message
s.EmitCard(pluginsdk.Card{...})      // 结构化卡片
```

Host 侧 `turnRenderer`（`cmd/host/main.go:366`）把它们翻译成 ANSI：

```go
case "markdown":   fmt.Print(mdansi.Render(text))
case "expandable": r.renderExpandable(title, body, detail, open)
case "message":    fmt.Println(prefix + text)
```

换 CLI / Web / GUI，插件**完全无感**——渲染介质本身也插件化了。

### 7.2 Agent Loop 是可替换 Capability（ADR-0003）

前面已述：`runTurn` 检测到有人提供 `loop` 就整轮外包：

```go
if hasExternalLoop {
    return s.runExternalTurn(loopOwner, sessionID, userInput)
}
```

`runExternalTurn` 只做一次 `loop.turn` 星型调用，把整个 Turn 的编排权交给插件。

### 7.3 甚至 Subagent 也是 Host 编排多个插件的组合

`RunSubagent`（`serve.go:1231`）由 Host 组合出：`session.create`（新子会话）+
再用 `runTurn(allowSubagent=false, extraSystem=...)` 跑一轮（防递归）。
即"子代理"是既有插件的**编排产物**，而非独立硬编码功能。

---

## 8. 代码地图与常量速查

### 源码结构

```
cmd/host/main.go        CLI 薄内核入口：flag 解析、REPL、渲染介质 turnRenderer
protocol/frame.go       Frame 线协议（长度前缀 JSON）
plugin/manifest.go      plugin.json 结构 + 校验
discovery/discovery.go  扫描插件目录（不启动）
assembly/assembly.go    用户装配配置 → Plan（交集）
serve/serve.go          运行时内核：launch / 星型路由 / Default Loop / Call
serve/waterfall.go      双链拦截（cancel/audit）+ 拦截器
pluginsdk/server.go     插件作者 SDK：Handle/Call/Emit/Serve
pluginsdk/presentation.go 渲染意图 API
render/mdansi/          Markdown → ANSI

plugins/echo           参考插件（最小）
plugins/session        session 能力（append-only log + derive）
plugins/llm-openai     llm 能力（OpenAI 兼容，SSE 流式）
plugins/fakellm        确定性假 LLM（测试 fixture）
plugins/filetools      tools 能力（read/write/edit/grep/glob）
plugins/echotool       tools 能力（fixture，附 Presentation 卡）
plugins/contextmanager system-prompt 能力（Prompt 段落组装）
plugins/interceptor    拦截器（allow/rewrite/reject）
plugins/consumer       星型跨插件调用 fixture
plugins/slow / crashonce / crashix  生命周期/超时/崩溃 fixture
```

### Capability 常量

| Capability | 常量位置 | 方法 |
|-----------|---------|------|
| `session` | `serve.SessionCap` | create / append / query / derive |
| `llm` | `serve.LLMCap` | complete（+ `chunk` evt） |
| `tools` | `serve.ToolsCap` | list / call |
| `system-prompt` | `serve.SystemPromptCap` | assemble / registerSegment / registerContext |
| `loop` | `serve.LoopCap` | turn |
| `interceptor` | `serve.InterceptorCap` | before |
| `presentation` | `serve.PresentationCap` | card / render / stream / status（广播 evt） |
| `agent` | `serve.AgentCap` | request / inject（**Host 自有**，不走路由表） |

### Session Log fact 类型

| type | 谁写 | 作用 |
|------|------|------|
| `turn_start` / `turn_end` | Host | 一轮边界 |
| `step_start` / `step_end` | Host | 一次模型跳转边界（reason: completed/tools/max_steps） |
| `request_header` | Host | provider/model/step/turn 快照（审计/复现） |
| `message` | Host / plugin | 模型可见消息（role: system/user/assistant） |
| `tool_call` | Host | assistant 的工具调用意图（meta.tool_calls） |
| `tool_result` | Host | 工具执行结果（meta.tool_call_id） |

### "一切皆插件" 的三条核心不变量

1. **一能力一提供者**：`provides` 冲突则拒绝启动。
2. **依赖启动时闭合**：每个 `consumes` 必须有 `provides` 提供，否则拒绝启动。
3. **上下文可从日志重建**：`agent/request` 强制 `derive() == claimed`，否则
   `session_invariant_violation`（ADR-0002）。

---

## 结语

**Host = 无状态的、可插拔能力的星型路由器。**

- 用 `plugin.json` 的 `provides/consumes` 描述能力图
- 用 `discovery → assembly → serve` 完成"发现 → 选择 → 挂载"
- 用 `Frame` 协议 + 星型路由让插件互不直连，从而让拦截/审计/超时/自愈统一插在路径上
- 用 `pluginsdk` 把协议细节藏起来，让"写一个能力"= 注册一个 handler

**"一切"包括**：LLM、Session、Tools、Context Manager、Interceptor、Presentation，
以及 **Agent Loop 本身**。连最核心的推理循环都退化为"没人提供 `loop` 时的兜底默认值"——
这就是"一切皆插件"最彻底的证据。
