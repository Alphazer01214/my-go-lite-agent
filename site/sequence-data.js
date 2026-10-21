/* LiteAgent 数据流转 · 序列图定义
 *
 * mermaid 源码与仓库 docs/*.md 里的 ```mermaid 块同格式，
 * 所以同一段文字既能贴进 GitHub，也能被 site/sequence.js 渲染。
 *
 * steps 的键是消息序号（1 起，只数消息，Note 不计数）。
 */

const LA_SEQ = {};

LA_SEQ.parts = [
  { id: 'p1', n: '一', title: '线协议：Frame 怎么闭环', lead: '先不谈业务，只看一条 Request 从发出到收到唯一终态之间，id 经历了什么改写。' },
  { id: 'p2', n: '二', title: '浏览器入口：一次点击怎么进 Host', lead: '浏览器的请求不是 Frame。Bridge 把它翻译成 Host Call，并且由 Host 注入可信来源。' },
  { id: 'p3', n: '三', title: '一次 Turn：主链路', lead: '从 loop.turn 到 State Push 的完整序列。注意 agent 不直接调用 model-context。' },
  { id: 'p4', n: '四', title: '三个关键旁路', lead: '流式增量、上下文组装、工具轮。这三条是理解数据怎么流动的核心。' },
  { id: 'p5', n: '五', title: '终态：取消、异常、刷新', lead: '每条在途 Call 都必须收敛到唯一终态，页面刷新也不例外。' },
  { id: 'p6', n: '六', title: 'Host 内务：注册、路由、生命周期', lead: 'Host 不解释业务，但它决定谁能被找到。' },
];

LA_SEQ.diagrams = {

  /* ---------- 一、线协议 ---------- */

  frame: {
    title: 'Frame 闭环：跨插件调用的 id 改写',
    where: 'docs/protocol.md 第 3 节 · internal/host/transport.go',
    lead: 'pending 的键是 Host 自己分配的 fwd-N，不是插件的 id。插件拿到的 Response 与归因事件，id 都被还原成它发出去时的样子。',
    mermaid: `sequenceDiagram
    participant A as Plugin A
    participant H as Host
    participant B as Plugin B
    A->>H: req id=agent-1
    H->>B: req id=fwd-1 from=A
    B-->>H: evt id=fwd-1
    H-->>A: evt id=agent-1
    B-->>H: res id=fwd-1
    H-->>A: res id=agent-1 from=B`,
    steps: {
      1: { t: '插件发起调用', b: 'SDK 自己生成 outbound call id（<插件名>-<序号>），写一条 req Frame。插件不填 from，from 由 Host 在投递时注入。', k: 'Frame.id = agent-1' },
      2: { t: 'Host 转发并改写 id', b: 'Host 分配内部键 fwd-1，覆盖 Frame.id，并注入真实来源 from=A。同时登记 pending[fwd-1]，里面记着调用方原始 id agent-1。', k: 'pending[fwd-1] = {frameID: agent-1, target: B, caller: A}' },
      3: { t: 'Provider 发归因事件', b: '事件必须带在途调用的 id，Host 才能找到它属于哪次 Call。整条插件链里只有 llm.chunk 一个这样的事件。', k: 'EmitWithID(req.ID, "llm", "chunk", ...)' },
      4: { t: 'Host 还原 id 后投递', b: 'Host 用 pending[fwd-1].frameID 把 id 换回 agent-1，投递给真正的发起方。事件和 Response 走同一条还原路径。', k: 'ev.ID = wt.frameID' },
      5: { t: '唯一终态', b: 'Provider 返回 Response。Host 删除 pending 项，把 id 还原成 agent-1，并把 from 置为真正的 Provider。', k: 'res.ID = wt.frameID, res.From = wt.targetPlugin' },
      6: { t: '回到调用方', b: '调用方的 SDK 按 agent-1 找到自己的 pending 槽，唤醒等待者。每条 Request 最终只得到一条 Response。', k: '每条 Request 只得到一条 Response' },
    },
  },

  cancel: {
    title: 'cancel：不产生独立 Response',
    where: 'internal/host/cancel.go · pluginsdk/plugin.go writeCancel',
    lead: 'cancel 只是把在途调用打断。原 Request 依然要有一条终态 Response —— 要么 Provider 真的返回，要么 Host 合成。',
    mermaid: `sequenceDiagram
    participant A as Plugin A
    participant H as Host
    participant B as Plugin B
    A->>H: req id=agent-1
    H->>B: req id=fwd-1
    A->>H: cancel id=agent-1
    H->>B: cancel id=fwd-1 reason=cancelled
    Note over H,B: cancel 不产生独立 Response，原 Request 仍需一条唯一终态
    B-->>H: res id=fwd-1
    H-->>A: res id=agent-1`,
    steps: {
      1: { t: '在途调用', b: '与上一张图相同，pending[fwd-1] 建立。', k: 'pending[fwd-1]' },
      2: { t: '转发', b: 'Provider 收到 id=fwd-1 的 req，SDK 用这个 id 建立 inbound handler 上下文。', k: 'inbound[fwd-1]' },
      3: { t: '调用方发 cancel', b: '插件用自己那次 outbound call 的 id 发 cancel。Host 只认原调用方：别的插件发同样的 id 会被静默丢弃，不写任何 Frame。', k: '只有 candidate.caller == from 才受理' },
      4: { t: 'Host 改写并转发', b: 'Host 把 cancel 的 id 改成 pending 键 fwd-1，转发给目标插件。reason 只允许 cancelled 或 deadline_exceeded，其它值一律归一化成 cancelled。', k: 'reason ∈ {cancelled, deadline_exceeded}' },
      5: { t: '宽限期兜底', b: 'Host 起一个 2 秒的宽限计时器。Provider 在宽限期内没回，Host 合成终态 Response：call_cancelled 或 call_deadline_exceeded。', k: 'DefaultCancelGrace = 2s' },
      6: { t: '终态 Response', b: '无论 cancel 走没走完，原 Request 的那一条 Response 一定会到，且 id 被还原成 agent-1。', k: '每条 Request 只得到一条 Response' },
    },
  },

  /* ---------- 二、浏览器入口 ---------- */

  browser: {
    title: 'Browser Call：浏览器请求怎么变成插件调用',
    where: 'internal/webui/bridge.go · internal/host/call.go',
    lead: '浏览器不能提交 from，也不能调用 host.* 能力。这两点都由 Bridge 强制。',
    mermaid: `sequenceDiagram
    participant UI as session-chat
    participant B as Web Bridge
    participant H as Host
    participant A as agent
    UI->>B: POST /bridge/call loop.turn
    B->>H: CallWithEvents from=webui
    H->>A: req loop.turn id=host-1
    A-->>H: res id=host-1
    H-->>B: res id=host-1
    B-->>UI: NDJSON res`,
    steps: {
      1: { t: '组件发起 Capability Call', b: '浏览器端 window.LiteAgent.call("loop","turn",payload) 发一个 POST。请求体里不允许出现 from，带了就直接 400。', k: 'from 字段存在即 400' },
      2: { t: 'Bridge 转成 Host Call', b: 'Bridge 调 Host 的 CallWithEvents，来源写死成字符串 webui。浏览器无法影响这个值。', k: 'from = "webui"（Bridge 硬编码）' },
      3: { t: 'Host 分配 id 并路由', b: 'Host 分配 host-1 作为 pending 键，按 (capability, method) 找到路由属主，写一条 req 给 agent。', k: 'pending[host-1]' },
      4: { t: '插件返回', b: 'agent 的 handler 返回结果，SDK 写一条 res，id 是收到的 host-1。', k: 'type = res' },
      5: { t: 'Host 收口', b: 'Host 删除 pending，把 id 还原成浏览器给的请求 id。', k: 'res.ID = 浏览器请求 id' },
      6: { t: '回浏览器', b: 'Bridge 用 NDJSON 回流：零到多个归因 Event，最后必须跟一条 Response。', k: '零到多个 evt + 一条 res' },
    },
  },

  /* ---------- 三、主链路 ---------- */

  turn: {
    title: '一次 Turn：从点击到 State Push',
    where: 'plugins/agent/turn.go · docs/project-map/OVERVIEW.md 第 4 节',
    lead: '这是全链路。agent 不直接调用 model-context —— 它发的是 llm.complete，model-context 作为 Consumer 在链上截住它。',
    mermaid: `sequenceDiagram
    participant UI as session-chat
    participant H as Host
    participant A as agent
    participant S as session
    participant M as model-context
    participant L as llm-openai
    participant T as tools
    UI->>H: loop.turn
    H->>A: req loop.turn
    A->>S: session.create
    S-->>A: session_id
    A->>S: session.info
    S-->>A: model / context_window
    A->>S: session.append turn_start + message(user)
    A->>S: session.derive
    S-->>A: messages
    A->>T: tools.list
    T-->>A: tools[]
    A->>S: session.append step_start + request_header
    A->>M: req llm.complete
    M->>S: session.derive
    M->>S: session.append system_prompt + runtime_context
    M->>L: Proceed llm.complete
    L-->>A: evt llm.chunk
    A->>S: session.stream.update
    S->>H: host.ui.command stream
    H->>UI: Command stream
    L-->>A: res llm.complete
    A->>S: session.append message(assistant) + llm_usage
    A->>S: session.append step_end + turn_end
    S->>H: host.ui.state chat/history/trace/dock
    H->>UI: State Push
    A-->>H: res loop.turn
    H-->>UI: res loop.turn`,
    steps: {
      1: { t: '浏览器发起', b: 'session-chat 组件调用 loop.turn。Host 按 (loop, turn) 路由到 agent，路由键里没有插件名。', k: '路由键 = (capability, method)' },
      2: { t: '创建或复用 Session', b: '没有 session_id 就新建。session.create 会记下 UI 选的 model 与 context_window；复用会话沿用创建时的选择。', k: 'session.create' },
      3: { t: '拿到 session_id', b: 'session 返回会话 id，后续所有 Fact 都挂在它上面。', k: 'Fact Log: ./sessions/<id>.jsonl' },
      4: { t: '读会话元数据', b: 'session.info 返回 model、context_window 和规范后的 Workspace。agent 用它写 llm.complete 的 meta。', k: 'session.info' },
      5: { t: '返回元数据', b: 'Workspace 由 Session 拥有，agent 不接受调用方传入的不一致路径。', k: 'workspace 校验在 agent 侧' },
      6: { t: '写 turn_start 与用户消息', b: 'Fact Log 只追加，不改写。turn_start 之后如果出错，Session 才不会卡在 running。', k: 'Fact: turn_start / message(role=user)' },
      7: { t: 'agent 自己先投影一次', b: 'agent 调 session.derive 拿基线消息。model-context 还会再 derive 一次，所以开了 model-context 时每步 derive 跑两遍。', k: 'session.derive（纯投影，不写 Fact）' },
      8: { t: '基线消息', b: 'derive 是纯函数：同样的 Fact 一定得到同样的消息列表。', k: '确定性投影' },
      9: { t: '查工具', b: 'tools.list 是弱依赖。失败不会让 Turn 失败，agent 会把原因写进 request_header.meta.degraded。', k: '失败 → degraded，不中断 Turn' },
      10: { t: '工具清单', b: '工具按 Generation 与 active 状态过滤。Provider 没起来的工具不会出现在清单里。', k: 'tools.list → tools[]' },
      11: { t: '写 step_start 与 request_header', b: 'request_header 记录本次请求的投影头，含降级原因。', k: 'Fact: step_start / request_header' },
      12: { t: '发起 llm.complete', b: 'agent 发的是 llm.complete。Host 选链：model-context 声明 consumes 该键且 priority 2000，所以它先接到这一跳。', k: 'Host 选 priority 最高的活跃 Consumer' },
      13: { t: 'model-context 重新投影', b: 'Consumer 先 derive 一遍当前 Fact Log。', k: 'session.derive' },
      14: { t: '写上下文 Facts', b: 'system prompt 与 runtime context 写成 message Fact，用 meta.kind 区分（system_prompt / runtime_context）。内容 hash 变了才写。', k: 'Fact: message(meta.kind=system_prompt / runtime_context)' },
      15: { t: 'Proceed 续传', b: 'Consumer 改写 payload 后调 Proceed。Host 把链位上限从 2000 降到 1999，下一跳是 llm-openai。', k: 'host.proceed，一次性生效' },
      16: { t: '流式增量', b: 'llm-openai 边收 SSE 边发 llm.chunk 归因事件。Host 还原 id 后投给 agent，agent 累计正文、reasoning 和 tool calls。', k: 'evt: llm.chunk' },
      17: { t: '推送流式快照', b: 'agent 把累计结果交给 session。Stream Snapshot 是临时态，不落盘。', k: 'session.stream.update' },
      18: { t: 'session 转成 Command', b: 'session 把增量做成 host.ui.command 推给 Host。', k: 'host.ui.command' },
      19: { t: '浏览器收到 Command', b: 'Command 是临时行为，不重放。浏览器断线期间丢掉的增量不会补发。', k: 'Command 丢帧不补发' },
      20: { t: 'llm.complete 终态', b: '上游结束，llm-openai 返回完整结果。', k: 'res: llm.complete' },
      21: { t: '结果落盘', b: 'assistant 正文和 llm_usage 写成 Fact。llm_usage 记录 API 自己的用量，dock 的计量只看它，不用本地估算。', k: 'Fact: message(assistant) / llm_usage' },
      22: { t: '写终态', b: 'step_end 与 turn_end。reason 取值：completed / tool_calls / max_steps / cancelled / error。', k: 'Fact: step_end / turn_end' },
      23: { t: 'session 重建 State', b: 'State 由 Fact 投影出来，所以刷新后能重建。revision 由插件自己维护，Host 不参与。', k: 'host.ui.state' },
      24: { t: 'State Push', b: '浏览器把 State 交给对应 Mount 的 setData。', k: 'kind = state' },
      25: { t: 'Turn 返回', b: 'agent 的 handler 返回，SDK 写 res。', k: 'res: loop.turn' },
      26: { t: '回浏览器', b: 'Bridge 把 Response 写进 NDJSON 流。', k: 'NDJSON 收尾' },
    },
  },

  /* ---------- 四、旁路 ---------- */

  stream: {
    title: '流式增量：归因事件怎么回到发起方',
    where: 'plugins/llm-openai/main.go:43 · plugins/agent/stream.go',
    lead: 'llm.chunk 是当前插件链里唯一一个归因事件。它带的是在途调用的 id，Host 负责把它还原后送回真正的发起方。',
    mermaid: `sequenceDiagram
    participant L as llm-openai
    participant H as Host
    participant A as agent
    participant S as session
    participant UI as session-chat
    L-->>H: evt llm.chunk id=fwd-9
    H-->>A: evt llm.chunk id=agent-1
    A->>A: streamAccumulator 累计
    A->>S: session.stream.update
    S->>H: host.ui.command stream
    H->>UI: Command stream delta`,
    steps: {
      1: { t: 'Provider 发事件', b: 'llm-openai 用 EmitWithID(req.ID, "llm", "chunk", payload)。payload 有三种 channel：content、reasoning、tool_call。', k: 'channel ∈ {content, reasoning, tool_call}' },
      2: { t: 'Host 还原 id', b: 'Host 用 pending[fwd-9].frameID 把 id 换回 agent-1，投递给发起方。事件顺序与调用方接收顺序一致。', k: 'ev.ID = wt.frameID' },
      3: { t: 'agent 累计', b: 'streamAccumulator 认识全部三种 channel。落盘前按 24 条或 100ms 的阈值批量 flush，避免一条 Fact 一条 IO。', k: 'flush 阈值：24 条 / 100ms' },
      4: { t: '交给 session', b: 'session.stream.update 是一个 Request，不是 Event。session 校验 update_id 单调，回退的旧更新会被丢掉。', k: '乱序更新返回 accepted:false, reason:stale' },
      5: { t: '转成 Command', b: 'session 在 33ms 的 tick 上把增量合并成一条 Command 推出去。', k: '合并窗口 33ms' },
      6: { t: '浏览器收到', b: '浏览器端 runCommand("stream", payload) 应用 delta。', k: 'kind = command' },
    },
  },

  context: {
    title: '上下文组装：Consumer 在链上截住 llm.complete',
    where: 'internal/host/pipeline.go · plugins/model-context/builder.go',
    lead: '这是整个系统里最反直觉的一段：agent 发的是 llm.complete，model-context 却在它到达 llm-openai 之前把它拦下来改写一遍。',
    mermaid: `sequenceDiagram
    participant A as agent
    participant H as Host
    participant M as model-context
    participant L as llm-openai
    participant S as session
    A->>H: req llm.complete priority=2147483647
    H->>M: req llm.complete priority=2000
    Note over H,M: Host 取 priority 不超过链位上限的最高优先活跃实例
    M->>S: session.derive
    M->>S: session.append system_prompt + runtime_context
    M->>S: session.derive
    M->>M: enforceBudget 必要时压缩
    M->>S: session.append context_snapshot
    M->>H: host.proceed
    H->>L: req llm.complete priority=0
    L-->>H: res llm.complete
    H-->>M: res
    M-->>A: res llm.complete`,
    steps: {
      1: { t: 'agent 发起 llm.complete', b: 'Host 收到入口请求时把 priority 置成 2147483647。这只是链位上限，不是任何插件的 Manifest priority。', k: 'Frame.priority = 2147483647' },
      2: { t: 'Host 选链', b: 'model-context 的 manifest 声明 consumes llm.complete，priority 2000。Host 在不超过链位上限的活跃实例里取 priority 最大的那个。', k: '选链规则：priority ≤ 链位上限，取最大' },
      3: { t: '链位上限下调', b: '选中 priority 2000 后，上限降为 1999。于是 llm-openai（priority 0）成为下一跳。', k: 'nextPriority = 2000 - 1' },
      4: { t: 'Consumer 投影', b: 'model-context 先 derive 一遍当前 Fact Log。', k: 'session.derive' },
      5: { t: '写上下文 Facts', b: 'system prompt 用 role=system，runtime context 用 role=user，两者都靠 meta.kind 区分。内容 hash 变了才写。', k: 'hash 变化才写 Fact' },
      6: { t: '重新投影', b: '写完再 derive 一遍，新的上下文才进得了投影。', k: 'session.derive' },
      7: { t: '预算控制', b: 'enforceBudget 用 min(配置上限, context_window) 做上限，超过阈值就压缩。压不动就报 context_budget_exceeded。', k: '触发阈值默认 0.8，最多压 3 轮' },
      8: { t: '写快照', b: 'context_snapshot 记录这次真正发给模型的消息顺序（surface）、消息 hash 和被截断的序号。', k: 'surface / messages_hash / truncated_seqs' },
      9: { t: 'Proceed', b: 'Consumer 把重写好的 payload 交给下一跳。Proceed 一次性生效，重复调用返回 flow_already_proceeded。', k: 'host.proceed' },
      10: { t: 'Provider 收到改写后的请求', b: 'llm-openai 收到的是 model-context 改写过的 messages。', k: 'payload.messages 已被替换' },
      11: { t: '结果逐跳回流', b: 'Response 沿请求路径逆序回流，每跳都可以转换结果。', k: '逆序回流' },
      12: { t: '回到 agent', b: 'agent 拿到的是最终结果。整条链上 agent 从头到尾没碰过 model-context。', k: 'agent → model-context 无直接调用' },
    },
  },

  tool: {
    title: '工具轮：tools 只做注册与分发',
    where: 'plugins/tools/main.go · plugins/file-tools/main.go',
    lead: 'tools 不是 Provider。它按 (target_capability, target_method) 再发一次 Host Call，真正的执行在 file-tools / shell-tools 那边。',
    mermaid: `sequenceDiagram
    participant A as agent
    participant T as tools
    participant F as file-tools
    participant S as session
    A->>T: tools.call name + arguments
    T->>T: 查注册表 + 校验 active 与 Generation
    T->>F: req file.read
    F-->>T: res content + meta
    T-->>A: res tools.call
    A->>S: session.append message(role=tool)
    A->>A: 下一个 step`,
    steps: {
      1: { t: 'agent 发 tools.call', b: 'payload 是工具名加参数。工具名必须匹配 ^[a-z][a-z0-9_]{0,63}$。', k: 'tools.call' },
      2: { t: '注册表校验', b: 'tools 查注册表，再通过 host.plugins 快照确认 Provider 还活着、目标方法还注册着、Generation 没变。', k: 'Generation 变了 → tool_target_unavailable' },
      3: { t: '按路由键分发', b: 'tools 用 target_capability.target_method 再发一次 Host Call，落到真正拥有该路由键的插件。', k: 'tools.call → Host Call' },
      4: { t: 'Provider 执行', b: 'file-tools 在 Workspace 范围内做实际读写，用 os.Root 防路径逃逸。', k: 'Workspace 作用域' },
      5: { t: '结果回 tools', b: '工具结果统一形状：content、is_error、truncated、meta。', k: 'Result 形状固定' },
      6: { t: '回 agent', b: '工具层的错误不中断 Turn。工具失败时 content 变成 "tool error: ..."，meta.is_error 置 true。', k: '工具错误不终止 Turn' },
      7: { t: '落盘', b: '工具结果写成 role=tool 的 Fact，成为下一轮投影的一部分。', k: 'Fact: message(role=tool)' },
      8: { t: '继续下一步', b: 'agent 回到 step 循环，重新投影再问模型，最多 max_steps 轮。', k: 'step_end(reason=tool_calls)' },
    },
  },

  /* ---------- 五、终态 ---------- */

  cancelturn: {
    title: 'Turn 取消：只影响一个 Session 的活动 Turn',
    where: 'plugins/agent/turn.go:185 · plugins/llm-openai/openai.go:78',
    lead: '取消必须显式调用。刷新或关闭页面不会隐式取消任何 Turn。',
    mermaid: `sequenceDiagram
    participant UI as session-chat
    participant H as Host
    participant A as agent
    participant L as llm-openai
    participant S as session
    UI->>H: loop.cancel session_id
    H->>A: req loop.cancel
    A->>A: turnState.markCancelled
    A->>L: llm.cancel call_id
    L->>L: cancel 上游 HTTP 请求
    A->>S: session.stream.update cancel_requested
    A->>S: session.append message(assistant) partial
    A->>S: session.append turn_end cancelled
    S->>H: host.ui.state
    H->>UI: State Push`,
    steps: {
      1: { t: '组件显式取消', b: 'session-chat 调 loop.cancel，带 session_id。刷新页面走的是完全不同的路径，不会触发取消。', k: 'loop.cancel' },
      2: { t: '路由到 agent', b: 'Host 按 (loop, cancel) 路由。', k: '路由键 = (loop, cancel)' },
      3: { t: '标记取消', b: 'agent 在 turnState 上打标记，同时取出活动 call_id。', k: 'turnState.markCancelled' },
      4: { t: '通知 Provider', b: 'agent 调 llm.cancel。llm-openai 用它中止上游 HTTP 请求：ctx 是通过 http.NewRequestWithContext 传下去的。', k: 'llm.cancel' },
      5: { t: '上游真的停了', b: '注意：call_id 还没注册时 cancel 会先被缓冲，Provider 注册时补上，避免取消一个还不存在的调用。', k: '未注册的 call_id 先缓冲' },
      6: { t: '推送取消请求', b: '先把 cancel_requested 推给 UI，让输入框先恢复。', k: 'session.stream.update' },
      7: { t: '保存半截草稿', b: '已经收到的正文按 partial assistant 落盘，不浪费。', k: 'Fact: message(role=assistant, 部分内容)' },
      8: { t: '写取消终态', b: 'turn_end 的 reason 是 cancelled。Session 不会停在 running。', k: 'turn_end(reason=cancelled)' },
      9: { t: 'State 重建', b: 'State 从 Fact 投影出来，所以取消后的界面和刷新后一致。', k: 'host.ui.state' },
      10: { t: '推到浏览器', b: '输入框恢复。', k: 'State Push' },
    },
  },

  error: {
    title: '异常收口：Turn 不会停在 running',
    where: 'plugins/agent/turn.go:410 · internal/host/transport.go',
    lead: 'error_code 非空时 payload 不作为结果。原始错误继续往上抛，不会被包装掉。',
    mermaid: `sequenceDiagram
    participant H as Host
    participant A as agent
    participant S as session
    participant UI as session-chat
    A->>S: session.append turn_start
    A->>H: req llm.complete
    H-->>A: res error_code 非空
    A->>S: session.append turn_end error
    S->>H: host.ui.state
    H->>UI: State Push
    A-->>H: res loop.turn error_code
    H-->>UI: res loop.turn error_code`,
    steps: {
      1: { t: 'turn_start 已写入', b: '这是收口能生效的前提。', k: 'Fact: turn_start' },
      2: { t: '发起 llm.complete', b: '走的是和正常 Turn 完全一样的链路。', k: 'req: llm.complete' },
      3: { t: '拿到错误终态', b: 'Response 带非空 error_code。payload 不作为结果，原始错误继续向上传。', k: 'error_code 非空 → payload 作废' },
      4: { t: '补写 turn_end', b: 'defer 收口器保证 turn_start 与 turn_end 成对。reason 是 error，meta.error 存原始错误。', k: 'turn_end(reason=error)' },
      5: { t: 'State 重建', b: '界面回到可用状态。', k: 'host.ui.state' },
      6: { t: '推给浏览器', b: '错误在 Chat 面板里可见。', k: 'State Push' },
      7: { t: '错误原样返回', b: 'agent 不吞错误，也不改写 error_code。', k: 'res: loop.turn' },
      8: { t: '回到浏览器', b: 'Bridge 把 error_code 交给浏览器端，LiteAgent.call 以它作为 error.code 抛出。', k: 'error.code = Frame.error_code' },
    },
  },

  refresh: {
    title: '刷新与重连：断开只停止写回',
    where: 'internal/webui/publisher.go · internal/webui/static/loader.mjs',
    lead: '页面刷新会断开 /bridge/ui。Host 不会因此取消任何在途 Call，也不缓存结果、不支持按 Call ID 重放。',
    mermaid: `sequenceDiagram
    participant UI as Browser
    participant B as Web Bridge
    participant H as Host
    participant S as session
    UI->>B: GET /bridge/bootstrap
    B-->>UI: plugins / mounts / slots / diagnostics
    UI->>B: GET /bridge/ui
    B-->>UI: hello 第 1 条 首次连接
    Note over UI,H: 刷新 = 连接断开，不是取消
    UI->>B: GET /bridge/ui 重连
    B-->>UI: hello 第 2 条 断线重连
    UI->>UI: onResync → mountLoader.resync
    UI->>H: resync 触发 session.state
    H->>S: req session.state
    S-->>H: state
    H-->>UI: State Push`,
    steps: {
      1: { t: '重新读 Bootstrap', b: 'Bootstrap 只有插件、Mount、Slot 和 Diagnostic 的元数据，没有任何业务状态。', k: 'GET /bridge/bootstrap' },
      2: { t: '拿到挂载清单', b: 'Shell 知道哪个插件的哪个组件挂在哪个 Slot。', k: 'Mount 身份 = plugin_name + mount_id' },
      3: { t: '打开推送流', b: 'GET /bridge/ui 是一条长连接。', k: 'GET /bridge/ui' },
      4: { t: '第一条 hello', b: '每条连接建立后 Host 先发一条 hello。第一条代表首次连接。', k: 'hello #1 = 首次连接' },
      5: { t: '断开不等于取消', b: '浏览器断开只停止写回。后端 Call 继续跑，Host 继续等它的唯一终态；写不进去的 Event 和 Response 直接丢弃。', k: 'Host 不缓存结果' },
      6: { t: '重连', b: 'SDK 按指数退避重连，最长 10 秒。', k: '退避 1s → 10s' },
      7: { t: '第二条 hello', b: '客户端把第一条之后的 hello 当成断线重连信号，触发 onResync。', k: 'hello #2 = 断线重连' },
      8: { t: 'Shell 重新同步', b: 'mountLoader.resync：有 resync 方法就调它；没实现就拆掉元素重新挂载，让 connectedCallback 再取一次数。', k: 'resync() 可选，缺省则重新挂载' },
      9: { t: '组件自己取数', b: 'resync 是组件自己的事，平台不代劳。', k: 'session.state' },
      10: { t: 'Host 路由', b: '和普通调用一样走 (capability, method)。', k: 'req: session.state' },
      11: { t: 'session 重建 State', b: 'State 由 Fact 投影，所以刷新前后拿到的内容一致。revision 由插件维护，必须跨进程重启仍递增，否则新状态会被当成旧值丢掉。', k: 'revision 由插件自己保证单调' },
      12: { t: 'State Push', b: '推送回到浏览器。断线期间丢掉的 Command 不会补发；State 可以重建。', k: 'Command 不重放，State 可重建' },
    },
  },

  /* ---------- 六、Host 内务 ---------- */

  register: {
    title: 'host.register：路由表的唯一写入口',
    where: 'internal/host/transport.go:166 · docs/protocol.md 第 5 节',
    lead: '插件不按名字互相寻址。Host 靠 (capability, method) 找到该把请求交给谁。',
    mermaid: `sequenceDiagram
    participant P as Plugin
    participant H as Host
    P->>H: host.register handlers[]
    H->>H: 写 routes[capability.method] = plugin
    H-->>P: res accepted[] / rejected[]
    Note over H: 跨插件同键冲突进 rejected，属主不变`,
    steps: {
      1: { t: '插件上报自己的 handler', b: 'payload 是 handlers 数组，每项含 capability 与 method，可选 name 与 description。name 与 description 只用于展示，不参与路由。', k: 'host.register' },
      2: { t: 'Host 写路由表', b: '这是路由表的唯一写入口。键是 capability.method，值是插件名。插件名不参与寻址，只在 from、日志和依赖图里出现。', k: 'routes["llm.complete"] = "llm-openai"' },
      3: { t: '返回受理结果', b: 'accepted 是被接受的键，rejected 是被拒的键。同一插件重复注册同一个键是 last-wins。', k: 'accepted[] / rejected[]' },
      4: { t: '冲突怎么处理', b: '跨插件抢同一个键时，先注册的那个保住属主，后一个进 rejected。例外：声明了 consumes 的插件进 accepted，但它只是 Consumer，仍然不抢占属主。', k: 'Consumer 不抢占 Owner' },
    },
  },

  lifecycle: {
    title: '插件生命周期：挂载、路由、退出',
    where: 'internal/host/lifecycle.go · internal/host/proc.go · internal/plugin/mount.go',
    lead: 'Host 不做业务，但它决定谁能被找到，以及找不到时返回什么。',
    mermaid: `sequenceDiagram
    participant H as Host
    participant P as Plugin
    H->>P: 启动进程 cmd.Dir = 插件目录
    P->>H: host.register
    H-->>P: res accepted
    P->>H: req capability.method
    H-->>P: res
    Note over H,P: 进程退出 → Host 合成 plugin_down`,
    steps: {
      1: { t: 'Host 启动进程', b: 'autostart 为真的插件是启动根，按 depends_on 做 DFS 后序装载。工作目录设为插件包目录，所以插件读自己的 config.json 是相对路径。', k: 'cmd.Dir = 插件包目录' },
      2: { t: '插件注册自己', b: '进程起来后第一件事就是 host.register。', k: 'host.register' },
      3: { t: '注册受理', b: '实例状态转成 active。', k: 'state = active' },
      4: { t: '正常调用', b: '路由表此时已经有这个键。', k: 'req: capability.method' },
      5: { t: '正常返回', b: '每条 req 恰好一条 res。', k: 'res' },
      6: { t: '进程退出', b: 'Host 把实例标成 failed，failureCode 是 plugin_down，并合成终态 Response 给所有以它为目标的 pending。调用方自己挂掉时，pending 直接丢弃，不回任何东西。', k: 'plugin_down' },
    },
  },

};

/* 每张图属于哪一部分。 */
LA_SEQ.diagramParts = {
  frame: 'p1', cancel: 'p1',
  browser: 'p2',
  turn: 'p3',
  stream: 'p4', context: 'p4', tool: 'p4',
  cancelturn: 'p5', error: 'p5', refresh: 'p5',
  register: 'p6', lifecycle: 'p6',
};

/* ---------- 支撑表格 ---------- */

LA_SEQ.frameFields = [
  ['version', '2', 'Host 与 SDK 写死', '线协议版本，与 plugin.json 的 protocol 独立演进'],
  ['id', 'agent-1 / fwd-1 / host-1', '发起方写，Host 转发时改写', 'pending 的键是 fwd-N，Response 与归因事件都按它还原'],
  ['type', 'req / res / evt / cancel', '发起方写', 'cancel 不产生独立 Response'],
  ['from', 'agent / session / webui', 'Host 注入', '插件出站 Frame 一律不填，浏览器也伪造不了'],
  ['context_id', 'ctx-1', '发起方写', '选执行上下文，进而选 Plugin Instance 与固定 Generation'],
  ['capability', 'llm', '发起方写', '路由键的一半'],
  ['method', 'complete', '发起方写', '路由键的一半'],
  ['payload', '业务 JSON', '发起方写', '对 Host 完全不透明'],
  ['reason', 'cancelled / deadline_exceeded', '发起方写', '只出现在 cancel Frame 上'],
  ['priority', '2147483647 → 2000 → 0', 'Host 管理', '链位上限。入口置成 2147483647，每跳降为 consumer.priority − 1'],
  ['error_code', '空字符串表示成功', 'Host 或 SDK 写', '非空时 payload 不作为结果'],
  ['error_msg', '空字符串表示成功', 'Host 或 SDK 写', '给人看的说明，不是契约'],
];

LA_SEQ.pendingKeys = [
  ['插件 → 插件', 'fwd-N', '插件自己的 outbound call id', 'Host 写 fwd-N，Response 与归因事件按 pending 里的 frameID 还原'],
  ['Host 发起（浏览器 Bridge）', 'host-N', '浏览器请求 id', 'Bridge 收到后改写成浏览器给的 id'],
  ['Proceed 续传', 'fwd-N', '当前跳的 Frame.id', '每 Proceed 一次开一条新的 pending'],
];

LA_SEQ.errPoints = [
  ['route_failed', 'Manifest 声明了 consumes 却无活跃 Consumer 时', 'Host 不降级到 Provider，避免绕过策略型 Consumer'],
  ['method_not_found', '路由表里没有属主，或 handler 不存在', '插件被停掉后路由键被摘除，后续调用就是这个码'],
  ['plugin_down', '插件进程退出', 'Host 合成终态 Response'],
  ['plugin_not_ready', '实例存在但状态不是 active', 'loading / stopping / failed / inactive'],
  ['call_cancelled', 'Go Context 被取消，或取消宽限期到了', 'cancel 之后由 Host 兜底合成'],
  ['call_deadline_exceeded', 'Go Context 超过期限', '同上'],
  ['flow_already_proceeded', '同一次 Call 重复 host.proceed', 'Proceed 一次性生效'],
  ['pipeline_loop', '流水线绕回已访问的插件实例', 'visited 去重'],
  ['pipeline_hops_exceeded', '跳数超过 64', '上限写死在 Host'],
  ['consume_priority_conflict', '同键上多个同优先级 Consumer', 'Host 不按注册顺序静默决胜'],
  ['ui_not_mounted', 'Mount 不存在，或没有浏览器连接、队列已满', '插件可以重试或降级'],
  ['ui_owner_mismatch', '插件向别的插件的 mount 推送', 'Host 校验 mount 属主'],
  ['context_binding_invalid', 'Provider 重启后 Generation 变了', '旧绑定失效，需要重新 bind'],
  ['handler_error', 'handler 没指定错误码', '普通 error 最终映射到它'],
];
