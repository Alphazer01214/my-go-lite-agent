/* LiteAgent 数据流转 · 序列图定义
 *
 * mermaid 源码与仓库 docs/*.md 里的 ```mermaid 块同格式，
 * 所以同一段文字既能贴进 GitHub，也能被 site/sequence.js 渲染。
 *
 * steps 的键是消息序号（1 起，只数消息，Note 不计数）。
 */

const LA_SEQ = {};

LA_SEQ.parts = [
  { id: 'p0', n: '零', title: '总览：完整数据流转', lead: '一张图看完一次 Turn：浏览器发起、模型请求、工具调用、sandbox 确认、文件写入、结果回模型和 UI 推送。' },
  { id: 'p1', n: '一', title: '线协议：Frame 怎么闭环', lead: '先不谈业务，只看一条 Request 从发出到收到唯一终态之间，id 经历了什么改写。' },
  { id: 'p2', n: '二', title: '浏览器入口：一次点击怎么进 Host', lead: '浏览器的请求不是 Frame。Bridge 把它翻译成 Host Call，并且由 Host 注入可信来源。' },
  { id: 'p3', n: '三', title: '一次 Turn：主链路', lead: '从 loop.turn 到 State Push 的完整序列。注意 agent 不直接调用 model-context。' },
  { id: 'p4', n: '四', title: '三个关键旁路', lead: '流式增量、上下文组装、工具轮。这三条是理解数据怎么流动的核心。' },
  { id: 'p5', n: '五', title: '插件层：裁决、确认与委派', lead: '沙箱在工具接缝上裁决并等待人工确认，subagent 把任务派给独立会话的子代理。两条链路都只经 Host 路由。' },
  { id: 'p6', n: '六', title: '终态：取消、异常、刷新', lead: '每条在途 Call 都必须收敛到唯一终态，页面刷新也不例外。' },
  { id: 'p7', n: '七', title: 'Host 内务：注册、路由、生命周期', lead: 'Host 不解释业务，但它决定谁能被找到。' },
];

LA_SEQ.diagrams = {

  /* ---------- 零、完整数据流转 ---------- */

  full: {
    title: '完整数据流转：一次带确认的工具 Turn',
    where: 'internal/host/ · plugins/agent/turn.go · plugins/tools/ · plugins/sandbox/confirm.go · plugins/file-tools/',
    lead: '这是端到端总览：浏览器发起 loop.turn，agent 组装上下文并请求模型；模型决定调用 file_write 后，sandbox 在 tools.call 上确认，允许后 tools 分发到 file-tools；写入结果回到模型，最终回答与 State 一起回到浏览器。',
    mermaid: `sequenceDiagram
    participant UI as session-chat
    participant H as Host
    participant A as agent
    participant S as session
    participant M as model-context
    participant L as llm-openai
    participant T as tools
    participant SB as sandbox
    participant U as sandbox-confirm
    participant F as file-tools
    UI->>H: req loop.turn
    H->>A: req loop.turn
    A->>S: session.create / info
    S-->>A: session_id + model + context_window
    A->>S: append turn_start + message(user)
    A->>M: req llm.complete
    M->>S: derive + append system_prompt / runtime_context
    M->>L: Proceed llm.complete
    L-->>A: evt llm.chunk + res llm.complete
    A->>T: tools.call name=file_write
    T->>SB: req tools.call prio=500
    SB->>U: ui.state pending[confirm]
    U->>SB: sandbox.decide allow
    SB->>S: append sandbox_decision(confirm → allow)
    SB->>T: Proceed tools.call
    T->>F: req file.write (same call_id)
    Note over SB,F: call_id 标记命中，跳过二次裁决
    F-->>T: res file.write
    T-->>A: res tools.call
    A->>S: append message(tool)
    A->>M: req llm.complete (next step)
    M->>L: Proceed llm.complete
    L-->>A: res llm.complete (final)
    A->>S: append assistant + llm_usage + step_end / turn_end
    S->>H: host.ui.state
    H->>UI: State Push
    A-->>H: res loop.turn
    H-->>UI: res loop.turn`,
    steps: {
      1: { t: '浏览器发起', b: 'session-chat 调用 loop.turn。浏览器不提交 from，Bridge 把 HTTP 请求翻译成 Host Call。', k: 'loop.turn' },
      2: { t: 'Host 路由', b: 'Host 按 (loop, turn) 找到 agent，分配内部 pending 键，再把 req 投过去。', k: 'pending[host-N]' },
      3: { t: '创建或读取会话', b: '没有 session_id 就创建；已有会话沿用创建时的 model 与 context_window。', k: 'session.create / info' },
      4: { t: '会话元数据', b: 'session 返回 id、模型和规范后的 Workspace；后续 Fact 与工具调用都绑定这个会话。', k: 'session_id + workspace' },
      5: { t: '记录用户输入', b: 'turn_start 与用户消息先落盘。日志只追加，后面出错也不会改写。', k: 'Fact: turn_start / message(user)' },
      6: { t: '请求模型', b: 'agent 发的是 llm.complete，不直接调用 model-context。Host 选链时先交给 priority 2000 的 Consumer。', k: 'llm.complete → model-context' },
      7: { t: '组装上下文', b: 'model-context 重新 derive，并写入当前 system prompt 与 runtime context Facts；内容 hash 变化时才写。', k: 'Fact: system_prompt / runtime_context' },
      8: { t: 'Proceed 到模型', b: 'Consumer 改写 payload 后 Proceed，链位上限降低，下一跳是 llm-openai。', k: 'host.proceed' },
      9: { t: '模型返回工具调用', b: '上游流式返回增量；模型在本步决定调用 file_write，agent 把 assistant 消息与 tool_calls 落盘。', k: 'evt llm.chunk → tool_calls' },
      10: { t: '发起工具调用', b: 'agent 把工具名与参数发给 tools.call。', k: 'tools.call name=file_write' },
      11: { t: 'sandbox 截住接缝', b: 'sandbox 是 tools.call 的 Consumer，priority 500；它读取 tools.list 的风险并做裁决。', k: 'allow / confirm / modify / deny' },
      12: { t: '推送确认面板', b: '命中 confirm 后阻塞调用，创建一次性令牌，把待确认列表推给右下角 sandbox-confirm。', k: 'host.ui.state → confirm' },
      13: { t: '用户允许一次', b: '用户点击允许，sandbox.decide 删除令牌并把决定交给等待中的 handler。', k: 'sandbox.decide allow' },
      14: { t: '记录确认结果', b: '先写 confirm 审计，再写 allow 终态审计；两次审计使用同一个 confirm_token。', k: 'Fact: sandbox_decision' },
      15: { t: '继续外层调用', b: '允许后同一次 tools.call 继续，不需要模型重发；Proceed 后下一跳是 tools。', k: 'Proceed tools.call' },
      16: { t: '分发到直连键', b: 'tools 用 target_capability.target_method 再发一次 Host Call，并带上同一个 call_id；sandbox 的 60 秒标记命中，跳过二次裁决。', k: 'file.write (same call_id)' },
      17: { t: 'Provider 执行', b: 'file-tools 在 Workspace 范围内原子写入，路径越界会被拒绝；返回工具结果包络。', k: 'file.write' },
      18: { t: '工具结果回 agent', b: '结果统一为 content、is_error、truncated、meta 四段，经 tools 回到 agent。', k: 'res tools.call' },
      19: { t: '写 tool Fact', b: 'agent 把工具结果写成 role=tool 的 Fact，成为下一轮投影的一部分。', k: 'Fact: message(tool)' },
      20: { t: '下一 step 再请求模型', b: 'agent 重新投影上下文，再次发 llm.complete；工具结果已经在消息序列里。', k: 'llm.complete (next step)' },
      21: { t: '再次 Proceed', b: 'model-context 再次经过，链位与第一次相同。', k: 'host.proceed' },
      22: { t: '模型给出最终回答', b: '这一次不再有 tool_calls，agent 拿到最终正文。', k: 'res llm.complete' },
      23: { t: '写终态 Facts', b: 'assistant 正文、llm_usage、step_end 与 turn_end 落盘；reason 为 completed。', k: 'Fact: assistant / llm_usage / turn_end' },
      24: { t: 'session 重建 State', b: 'session 从 Fact Log 投影 chat、history、trace 与 dock，revision 单调递增。', k: 'host.ui.state' },
      25: { t: 'State Push', b: '浏览器把 State 交给对应 Mount 的 setData；刷新后可以用同一份 Fact 重建。', k: 'kind = state' },
      26: { t: 'Turn 返回', b: 'agent 的 handler 返回，SDK 写 res。', k: 'res loop.turn' },
      27: { t: '回浏览器', b: 'Host 把 res 还原成浏览器请求 id，NDJSON 流结束。', k: 'NDJSON 收尾' },
    },
  },

  /* ---------- 一、线协议 ---------- */

  frame: {
    title: 'Frame 闭环：跨插件调用的 id 改写',
      where: 'docs/platform/protocol.md 第 3 节 · 见 internal/host/transport.go',
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
      where: 'plugins/agent/turn.go · 见 docs/platform/development.md',
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
      2: { t: 'Host 分配 id 并路由', b: 'Host 给这次调用分配内部 id，记进 pending，然后把 req 投给路由属主 agent。', k: 'pending[host-N]' },
      3: { t: '创建或复用 Session', b: '没有 session_id 就新建。session.create 会记下 UI 选的 model 与 context_window；复用会话沿用创建时的选择。', k: 'session.create' },
      4: { t: '拿到 session_id', b: 'session 返回会话 id，后续所有 Fact 都挂在它上面。', k: 'Fact Log: ./sessions/<id>.jsonl' },
      5: { t: '读会话元数据', b: 'session.info 返回 model、context_window 和规范后的 Workspace。agent 用它写 llm.complete 的 meta。', k: 'session.info' },
      6: { t: '返回元数据', b: 'Workspace 由 Session 拥有，agent 不接受调用方传入的不一致路径。', k: 'workspace 校验在 agent 侧' },
      7: { t: '写 turn_start 与用户消息', b: 'Fact Log 只追加，不改写。turn_start 之后如果出错，Session 才不会卡在 running。', k: 'Fact: turn_start / message(role=user)' },
      8: { t: 'agent 自己先投影一次', b: 'agent 调 session.derive 拿基线消息。model-context 还会再 derive 一次，所以开了 model-context 时每步 derive 跑两遍。', k: 'session.derive（纯投影，不写 Fact）' },
      9: { t: '基线消息', b: 'derive 是纯函数：同样的 Fact 一定得到同样的消息列表。', k: '确定性投影' },
      10: { t: '查工具', b: 'tools.list 是弱依赖。失败不会让 Turn 失败，agent 会把原因写进 request_header.meta.degraded。', k: '失败 → degraded，不中断 Turn' },
      11: { t: '工具清单', b: '工具按 Generation 与 active 状态过滤。Provider 没起来的工具不会出现在清单里。', k: 'tools.list → tools[]' },
      12: { t: '写 step_start 与 request_header', b: 'request_header 记录本次请求的投影头，含降级原因。', k: 'Fact: step_start / request_header' },
      13: { t: '发起 llm.complete', b: 'agent 发的是 llm.complete。Host 选链：model-context 声明 consumes 该键且 priority 2000，所以它先接到这一跳。', k: 'Host 选 priority 最高的活跃 Consumer' },
      14: { t: 'model-context 重新投影', b: 'Consumer 先 derive 一遍当前 Fact Log。', k: 'session.derive' },
      15: { t: '写上下文 Facts', b: 'system prompt 与 runtime context 写成 message Fact，用 meta.kind 区分（system_prompt / runtime_context）。内容 hash 变了才写。', k: 'Fact: message(meta.kind=system_prompt / runtime_context)' },
      16: { t: 'Proceed 续传', b: 'Consumer 改写 payload 后调 Proceed。Host 把链位上限从 2000 降到 1999，下一跳是 llm-openai。', k: 'host.proceed，一次性生效' },
      17: { t: '流式增量', b: 'llm-openai 边收 SSE 边发 llm.chunk 归因事件。Host 还原 id 后投给 agent，agent 累计正文、reasoning 和 tool calls。', k: 'evt: llm.chunk' },
      18: { t: '推送流式快照', b: 'agent 把累计结果交给 session。Stream Snapshot 是临时态，不落盘。', k: 'session.stream.update' },
      19: { t: 'session 转成 Command', b: 'session 把增量做成 host.ui.command 推给 Host。', k: 'host.ui.command' },
      20: { t: '浏览器收到 Command', b: 'Command 是临时行为，不重放。浏览器断线期间丢掉的增量不会补发。', k: 'Command 丢帧不补发' },
      21: { t: 'llm.complete 终态', b: '上游结束，llm-openai 返回完整结果。', k: 'res: llm.complete' },
      22: { t: '结果落盘', b: 'assistant 正文和 llm_usage 写成 Fact。llm_usage 记录 API 自己的用量，dock 的计量只看它，不用本地估算。', k: 'Fact: message(assistant) / llm_usage' },
      23: { t: '写终态', b: 'step_end 与 turn_end。reason 取值：completed / tool_calls / max_steps / cancelled / error。', k: 'Fact: step_end / turn_end' },
      24: { t: 'session 重建 State', b: 'State 由 Fact 投影出来，所以刷新后能重建。revision 由插件自己维护，Host 不参与。', k: 'host.ui.state' },
      25: { t: 'State Push', b: '浏览器把 State 交给对应 Mount 的 setData。', k: 'kind = state' },
      26: { t: 'Turn 返回', b: 'agent 的 handler 返回，SDK 写 res。', k: 'res: loop.turn' },
      27: { t: '回浏览器', b: 'Bridge 把 Response 写进 NDJSON 流。', k: 'NDJSON 收尾' },
    },
  },

  /* ---------- 四、旁路 ---------- */

  stream: {
    title: '流式增量：归因事件怎么回到发起方',
    where: 'plugins/llm-openai/openai.go · plugins/agent/stream.go',
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
    where: 'internal/host/pipeline.go · plugins/model-context/assemble.go',
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
      5: { t: '结果回 tools，再回 agent', b: '工具结果统一形状：content、is_error、truncated、meta。工具层的错误不中断 Turn：失败时 content 变成 "tool error: ..."，meta.is_error 置 true。', k: 'Result 形状固定' },
      6: { t: '落盘', b: '工具结果写成 role=tool 的 Fact，成为下一轮投影的一部分。', k: 'Fact: message(role=tool)' },
      7: { t: '继续下一步', b: 'agent 回到 step 循环，重新投影再问模型，最多 max_steps 轮。', k: 'step_end(reason=tool_calls)' },
    },

  },

  /* ---------- 五、插件层 ---------- */

  guard: {
    title: '沙箱裁决：工具接缝上的四态与确认',
      where: 'plugins/sandbox/seam.go · plugins/sandbox/confirm.go · plugins/sandbox/ui/main.js · internal/host/pipeline.go',
    lead: 'sandbox 是 tools.call 的 Consumer，也是 file.write 与 shell.execute 的 Consumer。命中 confirm 时调用会阻塞，等待右下角 sandbox-confirm；允许后同一次调用继续。tools 分发出去的直连那一跳用 call_id 标记跳过二次裁决。',
    mermaid: `sequenceDiagram
    participant A as agent
    participant T as tools
    participant SB as sandbox
    participant S as session
    participant U as sandbox-confirm
    participant F as file-tools
    A->>T: req tools.call name=file_write
    T->>SB: req tools.call prio=500
    SB->>T: req tools.list 查 risk
    T-->>SB: res tools[]
    SB->>S: req session.append sandbox_decision(confirm)
    SB->>U: ui.state pending[token]
    Note over SB,U: handler 阻塞，等待用户决定
    U->>SB: sandbox.decide allow
    SB->>S: req session.append sandbox_decision(allow)
    SB->>T: req tools.call Proceed
    T->>SB: req file.write prio=500 (same call_id)
    Note over SB: call_id 标记命中，跳过二次裁决
    SB->>F: req file.write Proceed
    F-->>T: res file.write
    T-->>A: res tools.call`,
    steps: {
      1: { t: 'agent 发起工具调用', b: 'payload 是工具名加参数。tools 查注册表，确认 Provider 还活着、目标方法还注册着、Generation 没变。', k: 'tools.call' },
      2: { t: 'Consumer 截住这一跳', b: 'sandbox 的 manifest 声明 consumes tools.call，priority 500。入口这一跳只认 Consumer，Provider 在这一跳不可选。', k: 'priority 越大越靠前' },
      3: { t: '查风险刻度', b: 'sandbox 读 tools.list 的 risk 字段。缓存 5 秒，失败时 1 秒重试。查不到就按未知风险 70 处理。', k: 'risk 0–100，未知按 70' },
      4: { t: '拿到工具清单', b: '工具表按名字和 (target_capability, target_method) 两个索引建。', k: 'riskTable' },
      5: { t: '写 confirm 审计', b: '命中 confirm 时先写一条 decision=confirm 的 sandbox_decision 事实，并创建一次性令牌。审计与被裁决的调用在同一条时间线上；session 缺席时退化为插件内存里的环形缓冲。', k: 'Fact: sandbox_decision(confirm)' },
      6: { t: '推送确认面板', b: '待确认列表推给 sandbox-confirm。面板显示工具、风险、理由、参数摘要、会话名与剩余时间；它固定在视口右下角，不占插槽布局。', k: 'host.ui.state → confirm' },
      7: { t: '四态之一', b: 'allow 原样 Proceed；confirm 阻塞等待；modify 改写 arguments 后 Proceed；deny 短路返回工具结果包络，meta.code 是 sandbox_denied。命中规则优先于风险分段；都没命中时按 risk 分段判定。默认分段是 ≤49 放行、≥50 确认。', k: 'allow / confirm / modify / deny' },
      8: { t: '人工决定', b: '用户点击允许后，sandbox.decide 删除令牌并把决定交给等待中的 handler；拒绝返回 sandbox_denied，超时返回 sandbox_confirm_expired，取消轮次会清理令牌并写 cancelled 审计。', k: 'sandbox.decide allow / deny' },
      9: { t: 'Proceed 到 tools', b: '允许后同一次调用继续，Proceed 走 host.proceed，一次性生效。链位上限降到 499，下一跳是 tools。', k: 'host.proceed' },
      10: { t: '直连键再进链', b: 'tools 分发对 file.write 的调用会重新进链，而 sandbox 也声明 consumes file.write。外层 tools.call 放行时记下的 call_id 标记命中，这一跳直接放行，不再询问或重复写审计。', k: '60s call_id 标记' },
      11: { t: 'Provider 执行', b: 'file-tools 在 Workspace 范围内做实际写入，用 os.Root 防路径逃逸。', k: 'Workspace 作用域' },
      12: { t: '结果回到 tools', b: '工具结果统一形状：content、is_error、truncated、meta。', k: 'file.write res' },
      13: { t: '结果回到 agent', b: 'deny 或确认拒绝时这里不会发生：sandbox 短路返回的包络带 is_error=true，模型能读到原因。', k: 'deny / confirm 拒绝不进下一跳' },
    },
  },

  delegate: {
    title: '子代理委派：独立会话跑一个回合',
      where: 'plugins/subagent/runner.go · plugins/subagent/tools.go · plugins/agent/turn.go',
    lead: 'subagent 插件只做一件事：建子会话、跑一个 loop.turn、把最终回答当成工具结果交回父代理。它自己没有推理循环。',
    mermaid: `sequenceDiagram
    participant A as agent
    participant T as tools
    participant SUB as subagent
    participant S as session
    participant M as model-context
    participant L as llm-openai
    A->>T: req tools.call name=subagent
    T->>SUB: req subagent.run
    SUB->>S: req session.info
    S-->>SUB: res workspace model depth
    SUB->>S: req session.create origin=subagent
    SUB->>S: req session.append subagent_spawned
    SUB->>A: req loop.turn 子会话
    A->>M: req llm.complete
    M->>L: req llm.complete Proceed
    L-->>A: res llm.complete
    A-->>SUB: res loop.turn 子代理最终回答
    SUB->>S: req session.append subagent_finished
    SUB-->>T: res subagent.run 工具结果包络
    T-->>A: res tools.call`,
    steps: {
      1: { t: '模型要求委派', b: 'subagent 插件通过 tools.sync 上架工具 subagent，risk 20，parallel_safe。工具定义里写清了怎么写自足 prompt。', k: 'tools.sync' },
      2: { t: '按目标能力分发', b: 'tools 用 target_capability.target_method 再发一次 Host Call，落到 subagent.run。', k: 'target = (subagent, run)' },
      3: { t: '读父会话', b: '拿 workspace、model、context_window 和父级 delegation_depth。父会话不可解析就是 subagent_parent_unresolved。', k: 'session.info' },
      4: { t: '得到父上下文', b: '子代理沿用父会话的工作区、模型与上下文窗口，不继承父对话。', k: '同一份 workspace' },
      5: { t: '建子会话', b: 'session.create 带 origin=subagent、parent_session_id、delegation_depth。深度超过上限返回 subagent_depth_exceeded，默认上限 1。', k: 'origin=subagent' },
      6: { t: '父会话写开始标记', b: 'subagent_spawned 是过程事实，不进模型上下文。它是父会话聊天区列出子代理入口的依据。', k: 'Fact: subagent_spawned' },
      7: { t: '子会话跑一个回合', b: '子代理用的还是 agent 那套循环：投影上下文、调模型、执行工具。工具面去掉 subagent 本身，所以它不能嵌套。', k: 'loop.turn' },
      8: { t: '子会话的模型调用', b: '走的是同一条 llm.complete 消费链。origin=subagent 时 model-context 会追加 subagent_sections 提示段。', k: '同一把路由键' },
      9: { t: 'Proceed 到 Provider', b: '和主会话完全一样，model-context 改写 payload 后 Proceed 给 llm-openai。', k: 'host.proceed' },
      10: { t: '上游返回', b: '子代理收到的也是同一份 Provider 结果，模型目录、掩码与错误处理都一样。', k: 'res llm.complete' },
      11: { t: '回合收口', b: '父轮次的取消会透传到子轮次。子代理失败时返回 is_error=true 的包络，不是协议错误。', k: '取消向下透传' },
      12: { t: '父会话写结束标记', b: 'subagent_finished 带 state、duration_ms、turn_count 与 usage。', k: 'Fact: subagent_finished' },
      13: { t: '工具结果包络', b: 'content 是子代理的最终回答，meta 带 child_session_id 与用量。父会话不再额外写一条结果消息事实。', k: 'content = 最终回答' },
      14: { t: '回到父代理', b: '父代理把它当成一次普通工具结果，追加成 role=tool 的 Fact，进入下一步投影。', k: 'role=tool' },
    },
  },

  /* ---------- 六、终态 ---------- */

  cancelturn: {
    title: 'Turn 取消：只影响一个 Session 的活动 Turn',
    where: 'plugins/agent/turn_cancel.go · plugins/llm-openai/openai.go',
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
    where: 'plugins/agent/turn.go 的 defer 收口器 · internal/host/transport.go',
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
      5: { t: '重连，断开不等于取消', b: '浏览器断开只停止写回。后端 Call 继续跑，Host 继续等它的唯一终态；写不进去的 Event 和 Response 直接丢弃。SDK 按指数退避重连，最长 10 秒。', k: 'Host 不缓存结果' },
      6: { t: '第二条 hello', b: '客户端把第一条之后的 hello 当成断线重连信号，触发 onResync。', k: 'hello #2 = 断线重连' },
      7: { t: 'Shell 重新同步', b: 'mountLoader.resync：有 resync 方法就调它；没实现就拆掉元素重新挂载，让 connectedCallback 再取一次数。', k: 'resync() 可选，缺省则重新挂载' },
      8: { t: '组件自己取数', b: 'resync 是组件自己的事，平台不代劳。', k: 'session.state' },
      9: { t: 'Host 路由', b: '和普通调用一样走 (capability, method)。', k: 'req: session.state' },
      10: { t: 'session 重建 State', b: 'State 由 Fact 投影，所以刷新前后拿到的内容一致。revision 由插件维护，必须跨进程重启仍递增，否则新状态会被当成旧值丢掉。', k: 'revision 由插件自己保证单调' },
      11: { t: 'State Push', b: '推送回到浏览器。断线期间丢掉的 Command 不会补发；State 可以重建。', k: 'Command 不重放，State 可重建' },
    },
  },

  /* ---------- 七、Host 内务 ---------- */

  register: {
    title: 'host.register：路由表的唯一写入口',
    where: 'internal/host/transport.go 的 hostRegister · docs/platform/protocol.md 第 5 节',
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
      3: { t: '返回受理结果', b: 'accepted 是被接受的键，rejected 是被拒的键。跨插件抢同一个键时先注册的那个保住属主，后一个进 rejected；声明了 consumes 的插件进 accepted，但它只是 Consumer，仍然不抢占属主。等 Consumer 当前不活跃时，入口这一跳跳过它，直接交给 Provider——只有 Consumer 与 Provider 都不存在，才是 method_not_found。', k: 'accepted[] / rejected[] · 缺席即放行' },
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
      3: { t: '注册受理', b: '实例状态转成 active，路由表里出现这个键。', k: 'state = active' },
      4: { t: '正常调用', b: '路由表此时已经有这个键。', k: 'req: capability.method' },
      5: { t: '正常返回，然后进程退出', b: '每条 req 恰好一条 res。进程退出后 Host 把实例标成 failed，failureCode 是 plugin_down，并合成终态 Response 给所有以它为目标的 pending。调用方自己挂掉时，pending 直接丢弃，不回任何东西。', k: 'plugin_down' },
    },
  },

};

/* 每张图属于哪一部分。 */
LA_SEQ.diagramParts = {
  full: 'p0',
  frame: 'p1', cancel: 'p1',
  browser: 'p2',
  turn: 'p3',
  stream: 'p4', context: 'p4', tool: 'p4',
  guard: 'p5', delegate: 'p5',
  cancelturn: 'p6', error: 'p6', refresh: 'p6',
  register: 'p7', lifecycle: 'p7',
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
  ['route_failed', 'Host 侧无法完成这次调用：执行上下文绑定失效、目标实例不是 active、或 Proceed 之后找不到下一跳', 'Consumer 缺席不产生这个码：那一跳被跳过，调用直接交给 Provider'],
  ['method_not_found', '路由表里没有属主，或 handler 不存在', '插件被停掉后路由键被摘除，后续调用就是这个码；Consumer 与 Provider 都不存在时也是它'],
  ['plugin_down', '插件进程退出', 'Host 合成终态 Response'],
  ['plugin_not_ready', '实例存在但状态不是 active', 'loading / stopping / failed / inactive；Proceed 之后 Provider 被停掉也会落到它'],
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

/* 插件自有错误码。它们不在 protocol/errors.go 里，但会写进 Frame.error_code。
   每个插件用独立前缀，避免和平台的 30 个稳定码撞名。 */
LA_SEQ.pluginCodes = [
  ['agent', 'llm_output_truncated / llm_content_filtered / llm_tool_calls_missing', '模型输出被上游截断、被内容过滤拦截、或声称有 tool_calls 却没返回'],
  ['subagent', 'subagent_parent_unresolved / subagent_depth_exceeded / subagent_spawn_failed / subagent_tools_unavailable', '父会话不可解析、委派深度超限、子会话创建失败、工具面读不到'],
  ['model-context', 'context_budget_exceeded / context_derive_failed / context_record_failed / model_context_not_found', '压缩后仍超预算、投影失败、写 Fact 失败、找不到对应会话'],
  ['tools', 'tool_not_found / tool_name_conflict / tool_owner_mismatch / tool_target_unavailable / tool_target_reserved / tool_definition_invalid / tool_source_invalid', '注册表与分发期的本地错误；目标 Provider 停了就是 tool_target_unavailable'],
  ['file-tools', 'path_escape / path_not_found / file_exists / file_conflict / edit_ambiguous / edit_not_found / file_binary / file_too_large', '路径与内容层面的本地错误；路径越界一律拒绝'],
  ['shell-tools', 'command_blocked / command_not_found / cwd_escape / cwd_invalid / env_invalid / process_start_failed', '进程启动前的本地错误；取消与超时另用 call_cancelled、call_deadline_exceeded'],
  ['sandbox', 'sandbox_denied / sandbox_confirm_token_invalid / sandbox_confirm_expired / sandbox_rule_invalid / sandbox_rule_not_found', 'sandbox_denied 与 sandbox_confirm_expired 写在工具结果包络的 meta.code 里，不是协议错误；token_invalid 是确认令牌不存在或已消费；rule_* 是规则写入期的校验失败'],
  ['session', 'session_not_found / workspace_mismatch / workspace_not_found / workspace_required', '会话与工作区定位失败'],
  ['llm-openai', 'llm_upstream', '上游 HTTP 失败统一成一个码，具体状态在 error_msg 里'],
];
