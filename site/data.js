/* LiteAgent 结构图览 · 内容数据
   全部取自仓库当前源码：README.md、ARCHITECTURE.md、CONTEXT.md、CONSTRAINT.md、
   docs/platform 下的平台文档、docs/design 下的插件设计稿、plugins/README.md
   与各插件 plugin.json，并与 protocol、internal/host、internal/webui、pluginsdk
   与 plugins 目录下各插件的实现逐条比对。
   插件业务术语只出现在对应插件契约中。 */

const LA = {};

LA.facts = [
  { k: '6', t: '平台模块', d: 'protocol、pluginsdk、host、plugin、webui、cmd' },
  { k: '9', t: '已发布插件', d: 'agent、session、llm-openai、model-context、tools、file-tools、shell-tools、sandbox、subagent' },
  { k: '11', t: 'Host 方法', d: 'host.register 是路由表唯一写入口' },
  { k: '30', t: '稳定 error_code', d: 'protocol/errors.go 定义，Host 实际产出其中大部分' },
  { k: '4', t: '条接缝', d: 'llm.complete、tools.call、file.write 系列、shell.execute' },
  { k: '0', t: 'Host 内的业务面板', d: '对话、工具、模型调用全在插件里' },
];

LA.heroWire = [
  { type: 'req', text: 'req loop.turn → agent' },
  { type: 'req', text: 'req session.derive → session' },
  { type: 'req', text: 'req llm.complete → model-context' },
  { type: 'req', text: 'req tools.call → sandbox' },
  { type: 'evt', text: 'evt llm.chunk {channel:"content"}' },
  { type: 'ui', text: 'ui.state session-dock {model, usage, context}' },
  { type: 'res', text: 'res loop.turn {usage} error_code:""' },
];

LA.layers = [
  { id: 'browser', kind: 'outside', name: 'Browser', path: '插件 UI 资源 + Host Shell',
    role: '展示与交互。Component 收 State、响应 Command、发起 Capability Call。',
    not: '不是应用状态属主。业务执行、业务状态和持久化都在后端 Plugin Runtime。',
    files: ['internal/webui/static/'] },
  { id: 'webui', kind: 'platform', name: 'internal/webui', path: 'Shell · Slot Registry · Loader · Bridge',
    role: 'Host Shell、Mount Bootstrap、静态资源和 Browser Bridge。',
    not: '不解释插件业务状态，不做插件业务状态存储。',
    files: ['internal/webui/server.go', 'internal/webui/bridge.go', 'internal/webui/publisher.go'] },
  { id: 'host', kind: 'platform', name: 'internal/host', path: 'Frame 路由内核',
    role: 'Plugin Instance、执行上下文、Frame 路由、消费链、pending 和外部 Call 入口。',
    not: '不含业务策略。payload 对它不透明。',
    files: ['internal/host/transport.go', 'internal/host/pipeline.go', 'internal/host/call.go'] },
  { id: 'plugin', kind: 'platform', name: 'internal/plugin', path: 'Manifest 发现',
    role: '扫描插件目录、解析 Manifest、计算 autostart 顺序。',
    not: '不运行业务逻辑。',
    files: ['internal/plugin/discovery.go', 'internal/plugin/mount.go'] },
  { id: 'protocol', kind: 'platform', name: 'protocol', path: '线契约',
    role: 'Frame 编解码、版本和稳定错误码。',
    not: '不定义业务数据。',
    files: ['protocol/frame.go', 'protocol/errors.go'] },
  { id: 'sdk', kind: 'platform', name: 'pluginsdk', path: '插件开发入口',
    role: '注册 Handler、Call、Event、Proceed、取消和插件 UI 驱动接口。只认 JSON 字段名，不提供共享的插件契约类型。',
    not: '不定义插件业务契约，不管页面布局，不知道具体有哪些插件。',
    files: ['pluginsdk/plugin.go', 'pluginsdk/webui.go'] },
  { id: 'cmd', kind: 'platform', name: 'cmd/liteagent-server', path: '可执行入口',
    role: '组装 Host 与 WebUI，管理 HTTP 生命周期。',
    not: '不做业务实现。',
    files: ['cmd/liteagent-server/server.go'] },
  { id: 'plugins', kind: 'ext', name: 'plugins/', path: '扩展层',
    role: '每个插件自己决定提供哪些 Capability、如何保存业务状态、哪些 Component 负责展示。',
    not: '插件之间不直连，跨插件调用必须经 Host 路由。',
    files: ['plugins/agent/', 'plugins/session/', 'plugins/llm-openai/', 'plugins/model-context/',
      'plugins/tools/', 'plugins/file-tools/', 'plugins/shell-tools/', 'plugins/sandbox/', 'plugins/subagent/'] },
];

LA.frameTypes = ['req', 'res', 'evt', 'cancel'];

LA.frameNotes = {
  req: 'Request 要求对端执行一次 capability.method。每条 Request 必须得到一条 Response。自调用合法。',
  res: 'Response 与 Request 使用同一个 id。成功结果放在 payload；失败时 error_code 非空，payload 不作为结果。',
  evt: 'Event 不建立调用闭环。带 id 的归因 Event 属于一次在途 Call；无 id 的广播 Event 被 Host 记录日志后丢弃。',
  cancel: 'cancel 取消一次在途 Request，不产生独立 Response。原 Request 最终仍产生唯一 Response。',
};

LA.capMethods = [
  'loop.turn', 'loop.cancel',
  'agent.config.get', 'agent.config.set', 'agent.settings.get', 'agent.settings.save',
  'session.create', 'session.append', 'session.query', 'session.derive', 'session.list',
  'session.info', 'session.state', 'session.messages', 'session.facts', 'session.dock',
  'session.select', 'session.close', 'session.refresh', 'session.update', 'session.delete', 'session.stream.update',
  'session.workspace.list', 'session.workspace.create', 'session.workspace.select',
  'session.workspace.current', 'session.workspace.rename', 'session.workspace.remove',
  'llm.complete', 'llm.cancel', 'llm.config.get', 'llm.config.set',
  'llm.models.list', 'llm.models.set', 'llm.settings.get', 'llm.settings.save',
  'model-context.build', 'model-context.inspect', 'model-context.compact',
  'tools.sync', 'tools.register', 'tools.unregister', 'tools.list', 'tools.call',
  'file.read', 'file.write', 'file.list', 'file.stat', 'file.search', 'file.glob', 'file.replace', 'file.patch',
  'shell.execute',
  'subagent.run',
  'sandbox.check', 'sandbox.list_rules', 'sandbox.set_rule',
  'sandbox.pending.list', 'sandbox.decide',
  'sandbox.settings.get', 'sandbox.settings.save',
  'host.register', 'host.proceed', 'host.plugins', 'host.ui.state', 'host.ui.command',
  'host.context.create', 'host.context.bind', 'host.context.close',
  'host.instance.create', 'host.instance.close', 'host.instance.restart',
];

/* protocol/errors.go 定义的全部 30 个稳定 error_code。
   note 标注代码实况：哪些是 Host 真会写进 Frame.ErrorCode 的，哪些是保留码。 */
LA.errCodes = [
  { code: 'host_closed', src: 'Host', msg: 'Host 已关闭' },
  { code: 'route_failed', src: 'Host', msg: '路由失败', note: 'Host 侧无法完成这次调用：执行上下文绑定失效、目标实例不是 active、或 Proceed 之后找不到下一跳' },
  { code: 'method_not_found', src: 'Host/SDK', msg: '方法不存在', note: '路由表里没有属主时也是它：no owner for capability.method' },
  { code: 'plugin_not_mounted', src: 'Host', msg: '插件未挂载', note: '实例 id 不在运行表里' },
  { code: 'plugin_down', src: 'Host', msg: '插件进程已退出', note: 'Host 合成终态，关闭插件时也用它' },
  { code: 'plugin_disabled', src: 'Host', msg: '插件已禁用', note: '保留码：当前代码路径没有写入 disabled 的地方' },
  { code: 'plugin_not_ready', src: 'Host', msg: 'Provider 尚未接收新 Call', note: '实例存在但状态不是 active' },
  { code: 'timeout', src: '预留', msg: '调用超时', note: 'Host 只把它当内部 Go 错误，不写进 Frame' },
  { code: 'call_cancelled', src: 'SDK/Host', msg: 'Go Context 已取消' },
  { code: 'call_deadline_exceeded', src: 'SDK/Host', msg: 'Go Context 超过期限' },
  { code: 'handler_error', src: 'SDK', msg: 'handler 未指定错误码', note: '普通 error 最终映射到它' },
  { code: 'server_closed', src: 'SDK/Host', msg: '连接或服务关闭' },
  { code: 'bad_payload', src: 'Host/插件', msg: 'JSON 契约错误' },
  { code: 'bad_arguments', src: '插件', msg: '参数不合法' },
  { code: 'frame_too_large', src: '传输', msg: 'Frame 超过 16 MiB 上限', note: '由编解码器产出，不是 Host 合成' },
  { code: 'capability_conflict', src: 'Host', msg: '能力属主冲突', note: '保留码：跨插件抢同一路由键实际由 host.register 的 rejected 列表表达' },
  { code: 'consume_priority_conflict', src: 'Host', msg: '同一 (capability, method) 上有多个同优先级 Consumer' },
  { code: 'flow_already_proceeded', src: 'SDK/Host', msg: '同一次 Call 重复 host.proceed' },
  { code: 'pipeline_loop', src: 'Host', msg: '流水线绕回已访问的插件实例' },
  { code: 'pipeline_hops_exceeded', src: 'Host', msg: '流水线跳数超过上限', note: '上限 64 跳' },
  { code: 'ui_not_mounted', src: 'Host/WebUI', msg: 'Mount 不存在，或当前没有浏览器连接、推送队列已满' },
  { code: 'ui_owner_mismatch', src: 'Host/WebUI', msg: 'Mount 属主不匹配', note: '插件不能向其他插件的 mount 推送' },
  { code: 'instance_not_found', src: 'Host', msg: 'Plugin Instance 不存在' },
  { code: 'instance_exists', src: 'Host', msg: 'Plugin Instance 标识已存在' },
  { code: 'instance_closed', src: 'Host', msg: 'Plugin Instance 已关闭' },
  { code: 'context_not_found', src: 'Host', msg: '执行上下文不存在' },
  { code: 'context_closed', src: 'Host', msg: '执行上下文已关闭' },
  { code: 'context_owner_mismatch', src: 'Host', msg: '执行上下文属主不匹配' },
  { code: 'context_participant_mismatch', src: 'Host', msg: '调用方不是上下文参与者' },
  { code: 'context_binding_invalid', src: 'Host', msg: 'Capability 实例绑定不合法', note: 'Provider 重启后 Generation 变更，旧绑定即失效' },
];

LA.wire = { prefix: '4-byte big-endian length', body: 'UTF-8 JSON body', max: 16 * 1024 * 1024, version: 2 };

LA.pipeline = {
  capability: 'llm',
  method: 'complete',
  owner: { name: 'llm-openai', priority: 0, note: 'provides llm，是路由属主 Owner，实际发起上游请求' },
  consumers: [
    { name: 'model-context', priority: 2000, note: 'Register("llm","complete")：写 system / runtime Facts，重新 derive，再 Proceed' },
  ],
  entry: { name: 'Host 入口', priority: 2147483647, note: 'Host 把入口 Request 的 priority 置为 2147483647。这是链位上限，不是任何插件的 Manifest priority。' },
  rules: [
    '第一跳只认 Consumer：manifest 声明 consumes 该键时，Provider 在入口这一跳不可选',
    '选下一跳：在 priority ≤ 当前链位上限的活跃实例里取 priority 最大者',
    '每跳把上限降为 consumer.priority − 1，Request 因此按 priority 递减传播',
    '同键出现相同最高 priority：consume_priority_conflict，Host 不按注册顺序静默决胜',
    'Proceed 一次性生效，重复调用返回 flow_already_proceeded',
    '跳数上限 64，越界返回 pipeline_hops_exceeded；绕回已访问实例返回 pipeline_loop',
    'Consumer 缺席即放行：入口这一跳找不到活跃 Consumer 时，调用直接交给该键的 Provider',
  ],
};

/* 消费链接缝全景。同一 (capability, method) 上的成员按 priority 递减排列。 */
LA.seams = [
  ['llm.complete', 'model-context（2000）→ llm-openai（0）', '组装上下文，改写 payload 后 Proceed；Consumer 缺席时基线直达 Provider'],
  ['tools.call', 'sandbox（500）→ tools（0）', '先裁决再分发；confirm 阻塞等待人工决定，tools 分发出去的那一跳会重新进链'],
  ['file.write / file.replace / file.patch', 'sandbox（500）→ file-tools（0）', '直连高危能力也过裁决；外层已放行的 call_id 会跳过二次裁决'],
  ['shell.execute', 'sandbox（500）→ shell-tools（0）', '命令执行也过裁决；外层已放行的 call_id 会跳过二次裁决'],
];

LA.pluginPos = {
  host: { x: 560, y: 58 },
  'subagent': { x: 300, y: 195 },
  agent: { x: 560, y: 195 },
  sandbox: { x: 880, y: 195 },
  session: { x: 130, y: 350 },
  'llm-openai': { x: 340, y: 350 },
  'model-context': { x: 570, y: 350 },
  tools: { x: 880, y: 350 },
  'file-tools': { x: 680, y: 500 },
  'shell-tools': { x: 1000, y: 500 },
};

LA.plugins = [
  { name: 'host', label: 'Host', caps: [], status: '平台内核',
    color: 'var(--plat)', toggleable: false,
    desc: '薄平台内核。发现插件、启动进程、路由 Frame、装配消费链、维护调用闭环、提供 WebUI 平台壳。',
    provides: [], dependsOn: [], requires: [], priority: null,
    methods: [['host.register', '写入路由表，唯一写入口'],
      ['host.proceed', '续传流水线到下一跳，一次性生效'],
      ['host.plugins', '插件与实例状态分组快照'],
      ['host.ui.state', '推送 UI State'],
      ['host.ui.command', '推送 UI Command'],
      ['host.context.create / bind / close', '执行上下文与 Capability 绑定'],
      ['host.instance.create / close / restart', 'Plugin Instance 生命周期']],
    mounts: [], note: 'Host 实际分派 11 个方法。docs/platform/protocol.md 第 5 节只列了 10 个，漏掉 host.proceed。' },

  { name: 'agent', label: 'agent', caps: ['loop', 'agent'], status: '当前实现',
    color: 'var(--teal)', toggleable: false,
    desc: 'Turn 编排。创建或复用 Session，解析模型，写 Fact，投影上下文，调模型，累计流式增量，执行工具，收口取消与异常。',
    provides: ['loop', 'agent'], dependsOn: ['session', 'llm-openai'],
    requires: ['session', 'llm', 'tools', 'model-context'], priority: null,
    methods: [['loop.turn', '跑一个 Turn；同一 Session 串行，不同 Session 并发'],
      ['loop.cancel', '取消该 Session 的活动 Turn'],
      ['agent.config.get / set', 'default_scheme 与 max_steps'],
      ['agent.settings.get / save', 'Settings Schema 读写']],
    mounts: [], note: '默认 scheme 是 tool_calling，chat scheme 不带工具。turn 收 allowed_tools / denied_tools 裁剪工具面；tool_parallelism=auto 且工具都标了 parallel_safe 时并发执行，上限 max_parallel_tools。模型 id 在会话创建时固定，目录里被删掉时回落到默认模型并写 model_fallback_from。只提供 Settings，不渲染聊天正文。' },

  { name: 'session', label: 'session', caps: ['session'], status: '当前实现',
    color: 'var(--green)', toggleable: false,
    desc: 'Session Fact Log、Workspace Registry、模型上下文投影和多会话 UI。拥有 Workspace 归属与当前全部顶层业务 Component。',
    provides: ['session'], dependsOn: [], requires: [], priority: 1000,
    methods: [['session.create', '创建 Session，保存 model / context_window'],
      ['session.append', '写 Fact 并推送 State'], ['session.derive', '纯投影模型上下文'],
      ['session.query / list / info', '读取历史与模型元数据'],
      ['session.state / dock / select / close / refresh', 'UI 会话管理'],
      ['session.update', '更新显示名称与置顶状态'],
      ['session.delete', '删除会话日志；有未闭合 Turn 时拒绝'],
      ['session.messages / facts', '按 before_seq 翻页读旧消息与旧 Fact'],
      ['session.stream.update', '接收流式快照'], ['session.workspace.*', 'Workspace 注册、选择与改名']],
    mounts: [['session-history', 'left'], ['session-chat', 'center'], ['session-trace', 'right'], ['session-dock', 'bottom']],
    note: 'Fact Log 落在 ./sessions/<id>.jsonl，只追加。历史列表支持改名、置顶与删除；置顶状态写入 session_meta 并跨重启恢复，删除前检查未闭合 Turn。子会话带 origin=subagent 与 parent_session_id，默认不出现在历史列表，父会话聊天区按 subagent_spawned 事实列出子代理入口，子会话是只读视图。sandbox_decision 与 subagent_spawned / subagent_finished 也写进同一份日志，它们不是 message，不进模型上下文。' },

  { name: 'llm-openai', label: 'llm-openai', caps: ['llm'], status: '当前实现',
    color: 'var(--rose)', toggleable: false,
    desc: '无状态 OpenAI-compatible Completion。流式与 JSON 两种模式，增量以归因事件 llm.chunk 返回。',
    provides: ['llm'], dependsOn: [], requires: [], priority: null,
    methods: [['llm.complete', '按调用方 model 选择流式或 JSON 模式，返回实际 model / context_window'],
      ['llm.chunk (evt)', '归因增量事件'], ['llm.cancel', '按 call_id 取消上游 HTTP 请求'],
      ['llm.config.get / set', '插件配置读写，api_key 回显时掩码'],
      ['llm.models.list / set', '模型目录与默认模型'],
      ['llm.settings.get / save', '插件详情设置：base_url、模型目录、默认模型、api_key（明文落盘）']],
    mounts: [], note: '取消会真的中止上游 HTTP 请求：llm.complete 把 ctx 交给 http.NewRequestWithContext。api_key 明文写在插件目录 config.json，读回时掩码。设置保存后 Host 派发 settings-saved，session-history 刷新模型下拉。上游 HTTP 失败统一成 llm_upstream。' },

  { name: 'model-context', label: 'model-context', caps: ['model-context'], status: '当前实现',
    color: 'var(--violet)', toggleable: true,
    desc: '透明上下文组装。作为 llm.complete 消费者，把 system prompt 与 runtime context 写成 Session Facts，再 derive、预算控制、快照与确定性压缩。',
    provides: ['model-context'], dependsOn: [], requires: ['session', 'tools'],
    consumes: ['llm.complete（priority 2000）'], priority: 2000,
    methods: [['llm.complete (Consumer, prio 2000)', 'prepare 后 Proceed 到 Provider'],
      ['model-context.build', '干跑下一次请求，不写 Fact'], ['model-context.inspect', '按 session 还原最新快照消息'],
      ['model-context.compact', '手工压缩']],
    mounts: [], note: 'manifest 没有 depends_on，只在运行时弱依赖 session 与 tools。预算取 min(max_context_tokens=60000, context_window)，超过 80% 触发压缩，最多 3 轮，压不动报 context_budget_exceeded。system_prompt 与 runtime_context 靠 meta.kind 区分，hash 变了才写。origin=subagent 的会话会追加 subagent_sections 提示段。' },

  { name: 'tools', label: 'tools', caps: ['tools'], status: '当前实现',
    color: 'var(--amber)', toggleable: true,
    desc: '通用 Tool 注册表与分发器。按 (capability, method) 分发到 Provider，不直连 Provider，不拥有 file.* 与 shell.* 路由。',
    provides: ['tools'], dependsOn: [], requires: [], priority: 0,
    methods: [['tools.sync', 'Provider 原子替换来源工具集'], ['tools.register / unregister', '增量注册与注销'],
      ['tools.list', '按来源 active、目标方法已注册、Generation 一致过滤'],
      ['tools.call', '按 target_capability.target_method 发起 Host Call']],
    mounts: [], note: '注册表缓存到 registry.json。工具名匹配 ^[a-z][a-z0-9_]{0,63}$，一个工具名只能属于一个来源插件。tools.call 分发出去的那一跳会重新进链，所以 sandbox 在 file.write 等键上能再截一次。缺 tools 时 agent 跳过工具阶段并在 request_header.meta.degraded 写入原因，Turn 不失败。' },

  { name: 'file-tools', label: 'file-tools', caps: ['file'], status: '当前实现',
    color: 'var(--amber2)', toggleable: true,
    desc: 'Workspace 范围内的文件读写八件套，带风险标注。',
    provides: ['file'], dependsOn: ['tools'], requires: ['tools'], priority: 0,
    methods: [['file.read / list / stat / search / glob', 'RiskLow'],
      ['file.write', '原子写入，RiskHigh'], ['file.replace / patch', '字面替换与有序编辑，RiskHigh']],
    mounts: [], note: '工具名 file_read … file_patch，TargetCapability 为 file。读类 risk 10 并标 parallel_safe，写类 risk 80。路径用 os.Root 限制在 Workspace 内；写类支持 expected_sha256 冲突检测。' },

  { name: 'shell-tools', label: 'shell-tools', caps: ['shell'], status: '当前实现',
    color: 'var(--amber2)', toggleable: true,
    desc: '同步命令执行与进程树取消。',
    provides: ['shell'], dependsOn: ['tools'], requires: ['tools'], priority: 0,
    methods: [['shell.execute', '工具名 shell_execute，RiskCritical']],
    mounts: [], note: 'risk 90。不解析 Shell 语法：管道、重定向、&& 与内建命令都不解释，要显式写解释器。取消时终止进程树（Windows 用 Job Object，Unix 用进程组），这是实现细节不是注册方法。交互式终端与后台 Shell 未实现。' },

  { name: 'sandbox', label: 'sandbox', caps: ['sandbox'], status: '当前实现',
    color: 'var(--rose)', toggleable: true,
    desc: '工具调用链上的权限裁决。只裁决，不执行工具。',
    provides: ['sandbox'], dependsOn: ['tools'], requires: ['session', 'tools'],
    consumes: ['tools.call', 'file.write', 'file.replace', 'file.patch', 'shell.execute（priority 500）'],
    priority: 500,
    methods: [['tools.call (Consumer, prio 500)', 'allow / confirm / modify / deny 四态裁决'],
      ['file.write / replace / patch (Consumer)', '直连高危能力也过裁决'],
      ['shell.execute (Consumer)', '直连命令执行也过裁决'],
      ['sandbox.check', '干跑一次裁决，不写审计'],
      ['sandbox.pending.list / decide', '列出待确认调用并提交人工决定'],
      ['sandbox.list_rules / set_rule', '规则集读写，set_rule 保持只升不降'],
      ['sandbox.settings.get / save', '默认动作、风险档、确认超时、审计保留与持久规则']],
    mounts: [['sandbox-confirm', 'right']],
    note: 'autostart 为 true，是启动根；depends_on tools 决定启动顺序。allow 原样 Proceed，modify 改写 arguments 后 Proceed，deny 短路返回工具结果包络（meta.code=sandbox_denied）。confirm 会阻塞当前调用：创建一次性令牌、写 confirm 审计、把待确认列表推给右下角 sandbox-confirm；用户允许后同一次调用继续，拒绝或超时返回错误包络，取消则清理令牌。默认分段是 risk ≤ 49 放行、≥ 50 确认。插件详情设置可直接增删改持久规则（允许删除 deny 规则），sandbox.set_rule 仍保持只升不降。tools.call 放行后，tools 会带同一个 call_id 再调直连键，sandbox 用 60 秒标记跳过第二遍裁决。缺席即放行是部署约定，不是平台保证。' },

  { name: 'subagent', label: 'subagent', caps: ['subagent'], status: '当前实现',
    color: 'var(--indigo)', toggleable: true,
    desc: '把一个自包含的任务委派给子代理，拿回它的最终结论。子代理有独立会话和自己的工具面。',
    provides: ['subagent'], dependsOn: [], requires: ['session', 'tools', 'loop'], priority: 0,
    methods: [['subagent.run', '委派一个任务并返回子代理的最终回复'],
      ['subagent（工具）', '作为工具暴露给 agent，供模型发起委派，risk 20，parallel_safe']],
    mounts: [], note: 'autostart 为 true，是启动根；requires session / tools / loop 都是弱依赖。子代理有独立会话：session.create 带 origin=subagent、parent_session_id、delegation_depth，深度上限默认 1，子代理工具面去掉 subagent 本身，所以不能嵌套。父会话写 subagent_spawned 与 subagent_finished 两条过程事实；子代理的最终回答只作为一次工具结果回到父代理。' },
];

LA.edges = [
  { from: 'agent', to: 'session', kind: 'depends' },
  { from: 'agent', to: 'llm-openai', kind: 'depends' },
  { from: 'file-tools', to: 'tools', kind: 'depends' },
  { from: 'shell-tools', to: 'tools', kind: 'depends' },
  { from: 'sandbox', to: 'tools', kind: 'depends' },
  { from: 'agent', to: 'tools', kind: 'requires', label: 'tools.list / call' },
  { from: 'agent', to: 'model-context', kind: 'requires', label: '仅声明，不直连' },
  { from: 'subagent', to: 'agent', kind: 'requires', label: 'loop.turn' },
  { from: 'subagent', to: 'session', kind: 'requires', label: '子会话与父会话事实' },
  { from: 'subagent', to: 'tools', kind: 'requires', label: '子代理自己的工具面' },
  { from: 'model-context', to: 'session', kind: 'requires', label: 'session.derive / append', dip: 150 },
  { from: 'model-context', to: 'llm-openai', kind: 'pipeline', label: 'llm.complete · prio 2000', labelDy: 62 },
  { from: 'sandbox', to: 'tools', kind: 'pipeline', label: 'tools.call · prio 500', bow: { dx: 46 }, labelAt: 0.62 },
  { from: 'sandbox', to: 'file-tools', kind: 'pipeline', label: 'file.write / patch', bow: { dx: -70 }, labelAt: 0.72 },
  { from: 'sandbox', to: 'shell-tools', kind: 'pipeline', label: 'shell.execute', bow: { dx: 110 }, labelAt: 0.72 },
];

LA.actors = [
  { id: 'ui', label: 'session-chat', sub: 'Browser' },
  { id: 'history', label: 'session-history', sub: 'Browser' },
  { id: 'dock', label: 'session-dock', sub: 'Browser' },
  { id: 'center', label: 'plugin center', sub: 'Host UI' },
  { id: 'bridge', label: 'Web Bridge', sub: 'Host' },
  { id: 'host', label: 'Host', sub: '路由' },
  { id: 'agent', label: 'agent', sub: 'plugin' },
  { id: 'session', label: 'session', sub: 'plugin' },
  { id: 'mc', label: 'model-context', sub: 'Consumer' },
  { id: 'llm', label: 'llm-openai', sub: 'Provider' },
  { id: 'api', label: 'Upstream', sub: 'LLM API' },
  { id: 'tools', label: 'tools', sub: 'registry' },
  { id: 'sb', label: 'sandbox', sub: 'Consumer' },
  { id: 'sbui', label: 'sandbox-confirm', sub: 'Browser' },
  { id: 'file', label: 'file-tools', sub: 'Provider' },
  { id: 'sub', label: 'subagent', sub: 'plugin' },
];

/* needs: 依赖的插件开关；altTo: 未满足时改写目标；skip: 未满足时的说明 */
LA.scenarios = {
  model: {
    label: '模型选择',
    steps: [
      { from: 'center', to: 'bridge', frame: 'http', cap: 'llm', method: 'settings.get', label: '插件中心 → 点击 llm-openai → 设置', note: 'Host Settings Renderer 读取 values，包括 models / default_model。' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'llm', method: 'settings.get' },
      { from: 'host', to: 'llm', frame: 'req', cap: 'llm', method: 'settings.get' },
      { from: 'llm', to: 'host', frame: 'res', cap: 'llm', method: 'settings.get', label: 'models / default_model / api_key configured' },
      { from: 'host', to: 'center', frame: 'res', cap: 'llm', method: 'settings.get', label: '渲染模型目录行与默认模型下拉' },
      { from: 'center', to: 'bridge', frame: 'http', cap: 'llm', method: 'settings.save', label: '编辑模型目录与默认模型', note: 'models/default_model 整体保存；api_key 明文写入 config.json。' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'llm', method: 'settings.save' },
      { from: 'host', to: 'llm', frame: 'req', cap: 'llm', method: 'settings.save' },
      { from: 'llm', to: 'host', frame: 'res', cap: 'llm', method: 'settings.save', label: '校验后写入 config.json' },
      { from: 'host', to: 'center', frame: 'res', cap: 'llm', method: 'settings.save', label: '保存成功，表单刷新' },
      { from: 'center', to: 'history', frame: 'note', cap: 'web', method: 'settings-saved', label: 'Host 派发 settings-saved', note: 'session-history 收到事件后重新读取模型目录。' },
      { from: 'history', to: 'bridge', frame: 'http', cap: 'llm', method: 'models.list', label: '刷新新会话模型下拉' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'llm', method: 'models.list' },
      { from: 'host', to: 'llm', frame: 'req', cap: 'llm', method: 'models.list' },
      { from: 'llm', to: 'host', frame: 'res', cap: 'llm', method: 'models.list', label: 'default_model + models + context_window' },
      { from: 'host', to: 'history', frame: 'res', cap: 'llm', method: 'models.list', label: '填充模型下拉' },
      { from: 'history', to: 'bridge', frame: 'http', cap: 'session', method: 'create', label: 'session.create {model, context_window}', note: '用户选择模型后点击新建会话。' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'session', method: 'create' },
      { from: 'host', to: 'session', frame: 'req', cap: 'session', method: 'create' },
      { from: 'session', to: 'host', frame: 'res', cap: 'session', method: 'create', label: 'session_start.meta 写入 model / context_window' },
      { from: 'host', to: 'history', frame: 'res', cap: 'session', method: 'create', label: '返回 session_id' },
      { from: 'session', to: 'dock', frame: 'ui', cap: 'host.ui', method: 'state', label: 'Dock State 显示会话模型', note: '会话被选中后 dock 显示模型与上下文窗口；用量仍为空。' },
    ],
  },
  normal: {
    label: '正常 Turn',
    steps: [
      { from: 'ui', to: 'bridge', frame: 'http', cap: 'loop', method: 'turn', label: 'session-chat 发起 Turn', note: '组件用 LiteAgent.call 发 Browser Request，浏览器不提交 from。' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'loop', method: 'turn', label: 'Bridge 转成 Host Call', note: 'serveBrowserCall → CallWithEvents，from 固定为 webui，由 Host 注入。' },
      { from: 'host', to: 'agent', frame: 'req', cap: 'loop', method: 'turn', label: '按 (loop, turn) 路由', note: '路由键不含插件名。Host 把入口 Request 的 priority 置为 2147483647 作为链位上限。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'create', label: '创建或复用 Session', note: '新建时记录 UI 选择的 model / context_window；复用会话沿用启动时选择。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'info', label: '读取 Workspace、模型与窗口', note: '返回 model / context_window，agent 随后写入 llm.complete meta。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '写 turn_start 与用户消息', note: 'Fact Log 只追加。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'derive', label: 'session.derive 投影基线', note: 'buildModelContext 构造 system / runtime / summary / shadow range 的纯投影。' },
      { from: 'agent', to: 'tools', frame: 'req', cap: 'tools', method: 'list', label: 'tools.list 查询可选工具', needs: ['tools'],
        skip: '缺 tools：跳过工具阶段，写入 request_header.meta.degraded', note: '弱依赖。capability 为 tools，工具按 Generation 与 active 过滤。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '写 step_start 与 request_header', note: 'request_header.meta.degraded 记录工具降级原因。' },
      { from: 'agent', to: 'mc', frame: 'req', cap: 'llm', method: 'complete', label: 'llm.complete 进入流水线', needs: ['model-context'],
        altTo: 'llm', altNote: '缺 model-context：不写 system / runtime Facts，agent 基线直接进入 llm-openai',
        note: '携带会话 model / context_window；model-context 以 priority 2000 注册为 Consumer，并按窗口约束压缩预算。' },
      { from: 'mc', to: 'llm', frame: 'req', cap: 'llm', method: 'complete', label: '写上下文 Facts 后 Proceed', needs: ['model-context'], skip: '缺 model-context：无 Proceed 一跳', note: 'Proceed 走 host.proceed，链位上限降到 2000 − 1，下一跳是 llm-openai。' },
      { from: 'llm', to: 'api', frame: 'http', cap: 'llm', method: 'complete', label: 'POST /chat/completions', note: 'llm-openai 是无状态 Provider，选择流式或 JSON 模式。' },
      { from: 'api', to: 'llm', frame: 'evt', cap: 'llm', method: 'chunk', label: 'SSE delta', note: '上游增量。' },
      { from: 'llm', to: 'agent', frame: 'evt', cap: 'llm', method: 'chunk', label: 'llm.chunk 归因事件', note: 'Host 还原调用方 id 后投递；Agent 累计正文、reasoning 和 tool calls。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'stream.update', label: 'session.stream.update', note: 'Stream Snapshot 是临时态，不落盘。' },
      { from: 'session', to: 'ui', frame: 'ui', cap: 'host.ui', method: 'command', label: 'Stream Command 推到 Chat Mount', note: 'host.ui.command，目标地址是 plugin_name + mount_id。' },
      { from: 'api', to: 'llm', frame: 'res', cap: 'llm', method: 'complete', label: 'final / [DONE]' },
      { from: 'llm', to: 'agent', frame: 'res', cap: 'llm', method: 'complete', label: 'completeOut 闭环' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: 'assistant 与 llm_usage 落盘', note: 'llm_usage 记录 API usage 与实际 model / context_window；落盘后清除 Stream Snapshot。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '写 step_end 与 turn_end' },
      { from: 'session', to: 'ui', frame: 'ui', cap: 'host.ui', method: 'state', label: 'Chat、History、Trace State', note: 'State 带插件定义的 revision，可被 Runtime 重建；ChatState 含 turns 摘要，用于消息折叠。' },
      { from: 'session', to: 'dock', frame: 'ui', cap: 'host.ui', method: 'state', label: 'Dock State 更新', note: '模型、会话累计 API 用量、最近 context_snapshot 投影到 session-dock；无选中会话时隐藏。' },
      { from: 'agent', to: 'host', frame: 'res', cap: 'loop', method: 'turn', label: 'loop.turn Response' },
      { from: 'host', to: 'ui', frame: 'res', cap: 'loop', method: 'turn', label: 'Bridge 返回浏览器', note: 'NDJSON：零到多个归因 Event 后必须有一条 Response。' },
    ],
  },
  tool: {
    label: '含工具轮',
    steps: [
      { from: 'agent', to: 'tools', frame: 'req', cap: 'tools', method: 'list', label: 'tools.list', needs: ['tools'], skip: '缺 tools：模型看不到任何工具' },
      { from: 'llm', to: 'agent', frame: 'evt', cap: 'llm', method: 'chunk', label: '返回 tool_calls', note: 'Agent 的 streamAccumulator 累计 tool calls。' },
      { from: 'agent', to: 'tools', frame: 'req', cap: 'tools', method: 'call', label: 'tools.call {target_capability, target_method}', needs: ['tools'], skip: '缺 tools：跳过工具阶段并标记 degraded' },
      { from: 'tools', to: 'file', frame: 'req', cap: 'file', method: 'read', label: '按 (capability, method) 分发', needs: ['file-tools'], skip: '缺 file-tools：工具没注册，tools.call 返回 tool_not_found' },
      { from: 'file', to: 'tools', frame: 'res', cap: 'file', method: 'read', label: 'Workspace 内文件内容', needs: ['file-tools'] },
      { from: 'tools', to: 'agent', frame: 'res', cap: 'tools', method: 'call', label: '工具结果', needs: ['tools'] },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '写 role=tool 的 Fact', note: '工具结果进入 Fact Log，成为下一步投影的一部分。' },
      { from: 'agent', to: 'mc', frame: 'req', cap: 'llm', method: 'complete', label: '下一 step 重新投影并请求模型', needs: ['model-context'], altTo: 'llm', altNote: '缺 model-context：基线直接进 llm-openai' },
    ],
  },
  guard: {
    label: '沙箱裁决与确认',
    steps: [
      { from: 'agent', to: 'tools', frame: 'req', cap: 'tools', method: 'call', label: 'tools.call {name:"file_write"}', needs: ['tools'], skip: '缺 tools：模型看不到任何工具' },
      { from: 'tools', to: 'sb', frame: 'req', cap: 'tools', method: 'call', label: 'Consumer 截住这一跳', needs: ['sandbox'],
        skip: '缺 sandbox：这一跳直接进 tools，不产生任何裁决', note: 'sandbox 声明 consumes tools.call，manifest priority 500。第一跳只认 Consumer，Provider 在入口这一跳不可选。' },
      { from: 'sb', to: 'tools', frame: 'req', cap: 'tools', method: 'list', label: '查工具风险刻度', note: 'sandbox 读 tools.list 的 risk 字段。查不到就按未知风险 70 处理。' },
      { from: 'sb', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '写 sandbox_decision 事实', note: 'confirm 时先写一条 decision=confirm 的审计条目，并创建一次性令牌。裁决条目与被裁决的调用落在同一条时间线上；session 缺席时退化为插件内存里的环形缓冲。' },
      { from: 'sb', to: 'sbui', frame: 'ui', cap: 'host.ui', method: 'state', label: '推送待确认列表', note: 'sandbox-confirm 固定在视口右下角，显示工具、风险、理由、参数摘要、会话与剩余时间。' },
      { from: 'sbui', to: 'sb', frame: 'http', cap: 'sandbox', method: 'decide', label: '用户允许一次', note: '拒绝时原调用返回 sandbox_denied；超时返回 sandbox_confirm_expired；取消轮次会清理令牌。允许后同一次调用继续，不需要模型重发。' },
      { from: 'sb', to: 'tools', frame: 'req', cap: 'tools', method: 'call', label: 'allow：原样 Proceed', note: 'modify 改写 arguments 后 Proceed；deny 直接短路返回，不进下一跳。默认分段是 risk ≤ 49 放行、≥ 50 确认。' },
      { from: 'tools', to: 'file', frame: 'req', cap: 'file', method: 'write', label: 'tools 分发到 file.write' },
      { from: 'file', to: 'sb', frame: 'req', cap: 'file', method: 'write', label: '同一 call_id 再进直连接缝', needs: ['sandbox'], skip: '缺 sandbox：file.write 直达 file-tools', note: 'tools 对外层 tools.call 放行后，会把同一个 call_id 带进直连键；sandbox 的 60 秒标记命中，跳过第二遍裁决与审计。' },
      { from: 'sb', to: 'file', frame: 'req', cap: 'file', method: 'write', label: 'Proceed 到 Provider' },
      { from: 'file', to: 'tools', frame: 'res', cap: 'file', method: 'write', label: '写入结果' },
      { from: 'tools', to: 'agent', frame: 'res', cap: 'tools', method: 'call', label: '工具结果回到 agent' },
    ],
  },
  delegate: {
    label: '子代理委派',
    steps: [
      { from: 'llm', to: 'agent', frame: 'evt', cap: 'llm', method: 'chunk', label: '模型要求调用 subagent 工具', note: 'subagent 插件通过 tools.sync 上架工具 subagent，risk 20，parallel_safe。' },
      { from: 'agent', to: 'tools', frame: 'req', cap: 'tools', method: 'call', label: 'tools.call {name:"subagent"}', needs: ['tools'], skip: '缺 tools：模型看不到委派工具' },
      { from: 'tools', to: 'sub', frame: 'req', cap: 'subagent', method: 'run', label: '按 target 能力分发', needs: ['subagent'], skip: '缺 subagent：工具没注册，tools.call 返回 tool_not_found' },
      { from: 'sub', to: 'session', frame: 'req', cap: 'session', method: 'info', label: '读父会话', note: '拿 workspace、model、context_window 与父级 depth。' },
      { from: 'sub', to: 'session', frame: 'req', cap: 'session', method: 'create', label: '建子会话', note: 'origin=subagent、parent_session_id、delegation_depth 一起写进 session_start。深度上限默认 1。' },
      { from: 'sub', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '父会话写 subagent_spawned', note: '过程事实，不进模型上下文。' },
      { from: 'sub', to: 'agent', frame: 'req', cap: 'loop', method: 'turn', label: '子会话跑一个回合', note: '子代理用同一个 agent 循环，工具面去掉 subagent 本身，所以不能嵌套。父轮次的取消会透传下来。' },
      { from: 'agent', to: 'mc', frame: 'req', cap: 'llm', method: 'complete', label: '子会话的 llm.complete', needs: ['model-context'], altTo: 'llm', altNote: '缺 model-context：基线直接进 llm-openai' },
      { from: 'agent', to: 'sub', frame: 'res', cap: 'loop', method: 'turn', label: '子代理最终回答' },
      { from: 'sub', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '父会话写 subagent_finished', note: '带 state、duration_ms、turn_count 与 usage。' },
      { from: 'sub', to: 'tools', frame: 'res', cap: 'subagent', method: 'run', label: '工具结果包络', note: 'content 是子代理的最终回答；失败时 is_error=true，不是协议错误。' },
      { from: 'tools', to: 'agent', frame: 'res', cap: 'tools', method: 'call', label: '回到父代理' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '父会话写 role=tool 的 Fact' },
    ],
  },
  inspect: {
    label: '上下文明细',
    steps: [
      { from: 'dock', to: 'bridge', frame: 'http', cap: 'model-context', method: 'inspect', label: '点击上下文 chip', note: 'session-dock 直接调用 model-context；请求携带 session_id。' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'model-context', method: 'inspect' },
      { from: 'host', to: 'mc', frame: 'req', cap: 'model-context', method: 'inspect' },
      { from: 'mc', to: 'session', frame: 'req', cap: 'session', method: 'query', label: '读取最新 context_snapshot 与消息 Fact', note: '只解析最新快照；surface 记录实际发送顺序。' },
      { from: 'session', to: 'mc', frame: 'res', cap: 'session', method: 'query', label: 'facts' },
      { from: 'mc', to: 'mc', frame: 'note', cap: 'model-context', method: 'reconstruct', label: '按 surface 还原消息', note: '截断工具结果按 truncated_seqs 生成同样的 stub，并用 messages_hash 校验；旧快照缺字段时用当前 derive 重试，hash 对上才采用。' },
      { from: 'mc', to: 'host', frame: 'res', cap: 'model-context', method: 'inspect', label: 'messages + breakdown + integrity' },
      { from: 'host', to: 'dock', frame: 'res', cap: 'model-context', method: 'inspect', label: '面板显示实际发送内容' },
      { from: 'dock', to: 'bridge', frame: 'http', cap: 'model-context', method: 'build', label: '切换到下一次预览', note: '预览不写 Fact；缺省工具时调用 tools.list。' },
      { from: 'bridge', to: 'host', frame: 'req', cap: 'model-context', method: 'build' },
      { from: 'host', to: 'mc', frame: 'req', cap: 'model-context', method: 'build' },
      { from: 'mc', to: 'tools', frame: 'req', cap: 'tools', method: 'list', label: '读取当前工具集', note: '仅 build 预览路径；llm.complete 路径不调用。' },
      { from: 'tools', to: 'mc', frame: 'res', cap: 'tools', method: 'list' },
      { from: 'mc', to: 'session', frame: 'req', cap: 'session', method: 'derive', label: '投影当前日志' },
      { from: 'session', to: 'mc', frame: 'res', cap: 'session', method: 'derive', label: '当前 surface' },
      { from: 'mc', to: 'host', frame: 'res', cap: 'model-context', method: 'build', label: '预览 messages' },
      { from: 'host', to: 'dock', frame: 'res', cap: 'model-context', method: 'build', label: '面板标注“尚未发送”' },
    ],
  },
  cancel: {
    label: '取消',
    steps: [
      { from: 'ui', to: 'host', frame: 'req', cap: 'loop', method: 'cancel', label: 'loop.cancel(session_id)', note: '取消业务必须显式调用；刷新或关闭页面不会隐式取消。' },
      { from: 'host', to: 'agent', frame: 'req', cap: 'loop', method: 'cancel', label: '路由到 agent' },
      { from: 'agent', to: 'agent', frame: 'note', cap: 'agent', method: 'internal', label: 'turnState.markCancelled()', note: 'Agent 内部标记，同时取出活动 call_id。' },
      { from: 'agent', to: 'llm', frame: 'req', cap: 'llm', method: 'cancel', label: 'llm.cancel {call_id}', note: 'llm-openai 用它中止上游 HTTP 请求；call_id 未注册时先缓冲，取消会在注册时补上。' },
      { from: 'agent', to: 'agent', frame: 'note', cap: 'agent', method: 'internal', label: 'runTurn 观察 done 走取消分支', note: 'llm.complete 的 Response 仍会回来，Agent 走取消分支收口。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'stream.update', label: 'stream.update(cancel_requested → cancelled)', note: '先把取消请求推给 UI，再推最终 cancelled 快照。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: '保存已累计的 partial assistant', note: '先结束 Turn，再保存流式草稿。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: 'append turn_end(cancelled)' },
      { from: 'session', to: 'ui', frame: 'ui', cap: 'host.ui', method: 'state', label: 'State Push，输入框恢复' },
    ],
  },
  error: {
    label: '异常收口',
    steps: [
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: 'turn_start 已写入' },
      { from: 'agent', to: 'llm', frame: 'req', cap: 'llm', method: 'complete', label: 'llm.complete' },
      { from: 'llm', to: 'agent', frame: 'res', cap: 'llm', method: 'complete', label: 'error_code 非空', note: '失败时 payload 不作为结果。原始错误继续向上传。' },
      { from: 'agent', to: 'session', frame: 'req', cap: 'session', method: 'append', label: 'append turn_end(error)', note: '保证 Session 不停留在 running。' },
      { from: 'session', to: 'ui', frame: 'ui', cap: 'host.ui', method: 'state', label: 'State Push' },
      { from: 'agent', to: 'host', frame: 'res', cap: 'loop', method: 'turn', label: '原始错误返回 Browser Call' },
    ],
  },
  refresh: {
    label: '刷新与重连',
    steps: [
      { from: 'ui', to: 'bridge', frame: 'note', cap: 'web', method: 'refresh', label: '页面刷新，/bridge/ui 连接断开', note: '浏览器断开只停止写回。' },
      { from: 'host', to: 'agent', frame: 'note', cap: 'host', method: 'continue', label: '后端 Call 继续', note: 'Host 不取消在途 Call，继续等待唯一终态。' },
      { from: 'host', to: 'bridge', frame: 'note', cap: 'host', method: 'drop', label: '写不进的 Event 与 Response 丢弃', note: 'Host 不缓存结果，也不支持按 Call ID 重放。' },
      { from: 'ui', to: 'bridge', frame: 'http', cap: 'bridge', method: 'bootstrap', label: 'GET / 与 /bridge/bootstrap', note: 'Shell 重新读取 Plugin、Mount、Slot 和 Diagnostic 快照。' },
      { from: 'bridge', to: 'ui', frame: 'http', cap: 'bridge', method: 'hello', label: '重连 /bridge/ui，Host 先发第二条 hello', note: '第一条 hello 表示首次连接，之后的表示断线重连。' },
      { from: 'ui', to: 'session', frame: 'req', cap: 'session', method: 'state', label: 'onResync → Component resync()', note: '未实现 resync 时 Shell 重新挂载，由 connectedCallback 重新取数。' },
      { from: 'dock', to: 'session', frame: 'req', cap: 'session', method: 'dock', label: 'session-dock resync', note: '只读取 DockState，不拉完整 Chat/Trace。' },
      { from: 'session', to: 'ui', frame: 'ui', cap: 'host.ui', method: 'state', label: 'Plugin Runtime 重建 State', note: 'revision 跨进程重启仍递增，否则新状态会被当成旧值丢弃。' },
      { from: 'session', to: 'dock', frame: 'ui', cap: 'host.ui', method: 'state', label: 'DockState 重建', note: '模型、API usage、上下文快照重新投影；没有选中会话则隐藏。' },
    ],
  },
};

LA.turnNote = '取消与异常只影响一个 Session 的活动 Turn。工具轮里的 file.* 与 shell.* 由 Provider 执行，tools 只做注册与分发；sandbox 在 tools.call 与 file.write、shell.execute 上各截一次，命中 confirm 时阻塞等待 sandbox-confirm 面板，允许后同一次调用继续；外层放行的 call_id 会让直连那一跳跳过二次裁决。子代理由模型通过 subagent 工具发起，它在独立会话里跑自己的 loop.turn，最终回答作为一次工具结果回到父代理。Turn 结束后的 Dock 用量来自 llm_usage；上下文明细通过 model-context.inspect 还原，下一次预览通过 build，不写 Fact。聊天区按 turns 摘要折叠过程：完成轮默认收起，最终回答与用户消息常显。';

LA.slots = {
  top: { tag: 'Host 控件', owner: 'Host', mount: '—', body: 'v1 的 top 由 Host 控件保留，插件使用其余四个 Slot。内容：插件中心入口（依赖图与选中插件详情：启停、实例、关系、设置）与 Diagnostic 悬浮窗。' },
  left: { tag: 'session-history', owner: 'session', mount: 'history', body: 'Workspace 分组、会话列表和新会话模型下拉。列表行支持改名、置顶与删除：改名走 session.update，置顶写入 session_meta 并跨重启恢复，删除走 session.delete 且运行中的会话会被拒绝。模型目录在插件中心 llm-openai 详情里维护；保存后通过 settings-saved 事件刷新下拉。子会话默认不出现在这个列表里。' },
  center: { tag: 'session-chat', owner: 'session', mount: 'chat', body: '多会话 Pane。发送调用 loop.turn，取消调用 loop.cancel；流式增量来自 Stream Command，落盘后由 State 重建。消息按 Turn 折叠：完成轮默认收起过程，保留用户消息和最终回答；Turn 头显示状态、过程条数、工具数和时长。父会话里按 subagent_spawned 事实列出子代理入口，子会话是只读视图。' },
  right: { tag: 'session-trace + sandbox-confirm', owner: 'session + sandbox', mount: 'trace + confirm', body: '同一 Slot 容纳两个 Mount。session-trace 展示 Fact Log 与上下文快照；sandbox_decision、subagent_spawned / subagent_finished 也落在这里，它们是平台事实，不进模型上下文。sandbox-confirm 固定在视口右下角，显示待确认工具、风险、理由、参数摘要、会话与剩余时间，提供允许一次、拒绝和查看会话。' },
  bottom: { tag: 'session-dock', owner: 'session', mount: 'dock', body: '模型、API usage 上下文仪表、会话累计用量和 Turn 状态。点击 chip 展开上下文与用量明细；context_snapshot 由 model-context 写入，消息通过 model-context.inspect 还原，下一次请求通过 build 预览。没有选中会话时 dock 与 bottom Slot 一并隐藏。' },
};

LA.stateSample = {
  kind: 'state',
  mount: { plugin: 'session', id: 'chat' },
  payload: { revision: 12, view: { panes: ['s-7', 's-8'], status: 'ready' } },
};

LA.commandSample = {
  kind: 'command',
  command: 'scroll',
  mount: { plugin: 'session', id: 'chat' },
  payload: { position: 'bottom' },
};

LA.componentApi = [
  ['setData(state)', 'State 决定 Component 当前是什么，应可由 Plugin Runtime 重建。'],
  ['runCommand(command, payload)', 'Command 决定 Component 此刻做什么：滚动、聚焦、临时动画。不是持久化记录。'],
  ['connectedCallback()', '在这里用 Capability Call 取初始数据。Component 不实现独立业务 API。'],
  ['disconnectedCallback()', 'Mount 被卸载时清理。'],
  ['resync()', '可选。重连后 Shell 调用它重读状态；未实现时 Shell 重新挂载组件。'],
];

LA.endpoints = [
  ['GET', '/', 'Host Shell'],
  ['GET', '/platform/...', 'Host Shell 自身的 JS/CSS'],
  ['GET', '/plugins/...', '插件 UI 资源目录'],
  ['GET', '/bridge/bootstrap', 'Plugin、Mount、Slot 和 Diagnostic'],
  ['POST', '/bridge/call', 'Capability Call，返回 NDJSON'],
  ['GET', '/bridge/ui', 'State、Command 和 Diagnostic 推送'],
  ['POST', '/bridge/pick-directory', '打开 Host 所在机器的文件夹选择框'],
  ['POST', '/bridge/plugins/rescan', '手动重新扫描 Manifest'],
  ['POST', '/bridge/plugins/{plugin}/start', '启动单个插件，校验硬依赖已 active'],
  ['POST', '/bridge/plugins/{plugin}/stop', '停止单个插件，先进入 stopping 等待旧 Call'],
];

/* session 的 Fact 词表（plugins/session/types.go）。
   type 是 Fact 类型；system_prompt / runtime_context 是 message Fact 的 meta.kind，
   不是独立的 Fact 类型。runtime_context 那条在代码里 role 是 user。
   host 标记的是平台事实：写进同一份日志，但永远不进模型上下文。 */
LA.ctxFacts = [
  { seq: 1, type: 'session_start', role: 'host', text: '创建 Session，绑定 workspace_id=w-3', always: true },
  { seq: 2, type: 'message', kind: 'system_prompt', role: 'system', text: 'system prompt 正文，hash 变化才写', mc: true },
  { seq: 3, type: 'message', kind: 'runtime_context', role: 'user', text: 'cwd、平台、时间、工具清单等 runtime context', mc: true },
  { seq: 4, type: 'turn_start', role: 'host', text: 'turn t-1 开始', always: true },
  { seq: 5, type: 'message', role: 'user', text: '读一下 README 的第一段', src: [5] },
  { seq: 6, type: 'request_header', role: 'host', text: '本次请求的投影头，含 meta.degraded', mc: true },
  { seq: 7, type: 'message', role: 'assistant', text: 'README 说明 LiteAgent 是进程外插件 Runtime……', src: [7] },
  { seq: 8, type: 'llm_usage', role: 'host', text: 'prompt / completion token 用量', always: true },
  { seq: 9, type: 'step_end', role: 'host', text: 'step 1 结束', always: true },
  { seq: 10, type: 'turn_end', role: 'host', text: 'turn t-1 结束，status=done', always: true },
  { seq: 11, type: 'message', role: 'user', text: '再列出 plugins/ 下的目录', src: [11] },
  { seq: 12, type: 'context_snapshot', role: 'host', text: '压缩前的上下文快照', compact: true },
  { seq: 13, type: 'context_compaction', role: 'host', text: 'enforceBudget 触发一次压缩', compact: true },
  { seq: 14, type: 'context_summary', role: 'host', text: 'shadow range：seq 4–10 由摘要替代', compact: true },
  { seq: 15, type: 'sandbox_decision', role: 'host', text: '一次裁决：decision / rule_id / risk / 参数摘要', host: true },
  { seq: 16, type: 'subagent_spawned', role: 'host', text: '子代理开始：child_session_id / delegation_depth', host: true },
  { seq: 17, type: 'subagent_finished', role: 'host', text: '子代理结束：state / duration_ms / turn_count / usage', host: true },
];

LA.security = [
  '默认只监听 127.0.0.1:2333；',
  '所有 POST 端点校验 Origin 同源，跨源请求返回 403；不带 Origin 的请求放行；',
  'UI 资源目录全部允许，不做 allowlist 或 MIME 白名单；',
  '浏览器不能调用 Host 平台管理方法；',
  '浏览器不能伪造 from；',
  'Host 校验 mount 属主；',
  'UI 发布权限由可信 from 和 Mount 属主校验确定；',
  'Secret 不回显；',
  'v1 插件 JS 是本机可信代码，不提供沙箱隔离。',
];

LA.rules = [
  ['路由只看 (capability, method)', '插件名不参与寻址，Provider 可被同能力实现替换。'],
  ['插件名只用于身份、日志和观测', '出现在 from、id 前缀和依赖图里，不进入路由表。'],
  ['插件之间不直连', '任何跨插件调用都经 Host，闭环与取消才可控。'],
  ['Host 不实现业务面板', '对话、工具、模型配置的面板归属对应插件。'],
  ['Plugin Runtime 驱动自己拥有的 Component', 'State 和 Command 只能发到自己的 mount。'],
  ['UI State 不替代插件自己的持久化', '刷新后不重放事件，State 必须可重建。'],
  ['插件私有结构体不得成为平台契约', '调用方在自己目录内维护适配类型。'],
  ['平台文档与插件文档分别维护', 'Host 文档不定义插件业务契约，插件文档不重写 Frame 协议。'],
  ['平台概念只有一个事实来源', '同一段代码在两个插件里长得一样，它就不属于插件。'],
  ['拆分的轴是关注点，不是技术类型', '按能力、阶段或界面分文件，不按 types/util/store 分。'],
  ['错误码字面量只在 errors.go', '其他文件用常量或具名构造器，全局唯一性可静态检查。'],
  ['缺席即放行是部署约定', '平台不提供强制拦截。sandbox 这类策略 Consumer 缺席时，调用照常到达 Provider。'],
];

LA.nongoals = ['多用户', '远程访问', '跨设备同步', '插件市场', '任意第三方代码沙箱', '调度配额', '在线迁移', '实例状态恢复'];
LA.removed = ['web-tools'];
LA.notImplemented = ['交互式终端', '后台 Shell', '沙箱确认流与确认面板', '进程级沙箱与网络出口控制'];

LA.cmds = [
  ['go test ./...', '运行平台测试'],
  ['go vet ./...', '静态检查，--verify 会先跑它'],
  ['go build ./cmd/liteagent-server', '构建 Host'],
  ['powershell -File scripts\\build.ps1 -Verify', '构建 dist 布局，PowerShell 参数是 -Clean / -List / -Verify / -Only'],
  ['bash scripts/build.sh --clean --verify', 'Bash 版参数是 --clean / --list / --verify / --only'],
  ['LITEAGENT_HTTP_ADDR=127.0.0.1:2333', '覆盖监听地址'],
];

LA.searchOrder = ['可执行文件旁的 plugins/；', '当前工作目录下的 plugins/。'];

LA.docMap = [
  ['ARCHITECTURE.md', 'Host 与 Plugin 的总体结构、六个平台模块与 v1 范围'],
  ['CONTEXT.md', '平台术语表，含 Avoid 用词'],
  ['CONSTRAINT.md', '文档与代码的十五条硬约束'],
  ['docs/platform/protocol.md', 'Frame、Host 方法、pending 与错误码'],
  ['docs/platform/development.md', '平台模块、结构体和图解'],
  ['docs/platform/pluginsdk.md', '插件开发接口'],
  ['docs/platform/manifest.md', 'Manifest、UI 与 Settings 静态契约'],
  ['docs/platform/webui.md', 'Host WebUI 与 Browser Bridge'],
  ['docs/platform/plugin-dev.md', '插件包结构与开发流程'],
  ['docs/platform/plugin-standard.md', '插件目录组织、文件划分与拆分轴'],
  ['docs/platform/build.md', '构建流程、增量缓存与 dist 布局'],
  ['docs/design/composition-model-context-tools-sandbox.md', '消费链组合方案与缺席语义'],
  ['docs/design/plugins/', '各插件的设计契约，sandbox 与 subagent 在 08、09'],
  ['docs/prompts/', '模型侧文本的规格：主 prompt、工具描述、压缩、子代理'],
  ['plugins/<name>/', '该插件的方法、payload、状态与前端交互'],
];

LA.reading = [
  'cmd/liteagent-server/server.go — 组装顺序，UI Publisher 必须在挂载前接好；',
  'protocol/frame.go — Frame 结构、版本与 16 MiB 上限；protocol/errors.go — 30 个稳定错误码；',
  'internal/host/transport.go — routeFrame、hostRegister、pending 与缺席放行；',
  'internal/host/pipeline.go — priority 选链、Proceed 与流水线终态；',
  'internal/host/call.go — Host 发起 Call 的闭环与 Execution Context 绑定；',
  'internal/webui/bridge.go — Browser Call 到 Host Call 的转换；',
  'internal/webui/publisher.go — State 与 Command 的推送信封；',
  'pluginsdk/plugin.go — Call、Emit、Proceed 与取消；',
  'plugins/agent/turn.go — buildModelContext、runTurn 与终态收口；',
  'plugins/agent/turn_tools.go — 工具面裁剪、风险刻度与并行执行；',
  'plugins/session/ui_chat.go — Fact 到 ChatState 的重建与子代理入口；',
  'plugins/model-context/assemble.go — 上下文 Facts 与 derive；',
  'plugins/model-context/budget.go — 预算与压缩；',
  'plugins/tools/registry.go — 工具注册、Generation 与来源归属；',
  'plugins/sandbox/seam.go — 四态裁决、直连接缝与 Proceed；',
  'plugins/sandbox/confirm.go — 阻塞确认、令牌与审计；',
  'plugins/sandbox/policy.go — 规则命中优先于风险分段；',
  'plugins/sandbox/ui/main.js — 右下角确认面板；',
  'plugins/subagent/runner.go — 子会话创建、委派深度与结果回写；',
  'plugins/llm-openai/openai.go — 流式与 JSON 两种上游模式。',
];
