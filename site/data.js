/* LiteAgent 结构图览 · 内容数据
   全部取自仓库文档与 plugin.json：README.md、ARCHITECTURE.md、CONTEXT.md、
   docs/protocol.md、docs/webui.md、plugins/README.md、docs/project-map/。 */

const LA = {};

LA.facts = [
  { k: '6', t: '平台模块', d: 'protocol、pluginsdk、host、plugin、webui、cmd' },
  { k: '8', t: '已发布插件', d: 'agent、session、llm-openai、model-context、tools、file-tools、shell-tools、sandbox' },
  { k: 'v2', t: 'Frame 版本', d: '线协议与 Manifest 版本独立演进' },
  { k: '16 MiB', t: '单条 Frame 上限', d: '4 字节大端长度 + UTF-8 JSON body' },
  { k: '30', t: '稳定 error_code', d: 'protocol/errors.go 定义，Host 实际产出其中大部分' },
  { k: '0', t: 'Host 内的业务面板', d: '对话、工具、模型调用全在插件里' },
];

LA.heroWire = [
  { type: 'req', text: 'req loop.turn → agent' },
  { type: 'req', text: 'req session.info {model, context_window} → session' },
  { type: 'req', text: 'req llm.complete → model-context' },
  { type: 'evt', text: 'evt llm.chunk {channel:"content"}' },
  { type: 'ui', text: 'ui.command scroll {position:"bottom"}' },
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
    role: 'Plugin Instance、执行上下文、Frame 路由、pending 和外部 Call 入口。',
    not: '不含业务策略。payload 对它不透明。',
    files: ['internal/host/transport.go', 'internal/host/call.go', 'internal/host/pipeline.go'] },
  { id: 'plugin', kind: 'platform', name: 'internal/plugin', path: 'Manifest 发现',
    role: '扫描插件目录、解析 Manifest、计算 autostart 顺序。',
    not: '不运行业务逻辑。',
    files: ['internal/plugin/discovery.go', 'internal/plugin/mount.go'] },
  { id: 'protocol', kind: 'platform', name: 'protocol', path: '线契约',
    role: 'Frame 编解码、版本和稳定错误码。',
    not: '不定义业务数据。',
    files: ['protocol/frame.go', 'protocol/errors.go'] },
  { id: 'sdk', kind: 'platform', name: 'pluginsdk', path: '插件开发入口',
    role: '注册 Handler、Call、Event、取消和插件 UI 驱动接口。',
    not: '不定义插件业务契约，不管页面布局。',
    files: ['pluginsdk/plugin.go', 'pluginsdk/webui.go'] },
  { id: 'cmd', kind: 'platform', name: 'cmd/liteagent-server', path: '可执行入口',
    role: '组装 Host 与 WebUI，管理 HTTP 生命周期。',
    not: '不做业务实现。',
    files: ['cmd/liteagent-server/server.go'] },
  { id: 'plugins', kind: 'ext', name: 'plugins/', path: '扩展层',
    role: '每个插件自己决定提供哪些 Capability、如何保存业务状态、哪些 Component 负责展示。',
    not: '插件之间不直连，跨插件调用必须经 Host 路由。',
    files: ['plugins/agent/', 'plugins/session/', 'plugins/llm-openai/', 'plugins/model-context/'] },
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
  'session.select', 'session.close', 'session.refresh', 'session.update', 'session.stream.update',
  'session.workspace.list', 'session.workspace.create', 'session.workspace.select',
  'session.workspace.current', 'session.workspace.rename', 'session.workspace.remove',
  'llm.complete', 'llm.cancel', 'llm.config.get', 'llm.config.set',
  'llm.models.list', 'llm.models.set', 'llm.settings.get', 'llm.settings.save',
  'model-context.build', 'model-context.inspect', 'model-context.compact',
  'tools.sync', 'tools.register', 'tools.unregister', 'tools.list', 'tools.call',
  'file.read', 'file.write', 'file.list', 'file.stat', 'file.search', 'file.glob', 'file.replace', 'file.patch',
  'shell.execute',
  'host.register', 'host.proceed', 'host.plugins', 'host.ui.state', 'host.ui.command',
  'host.context.create', 'host.context.bind', 'host.context.close',
  'host.instance.create', 'host.instance.close', 'host.instance.restart',
];

/* protocol/errors.go 定义的全部 30 个稳定 error_code。
   note 标注代码实况：哪些是 Host 真会写进 Frame.ErrorCode 的，哪些是保留码。 */
LA.errCodes = [
  { code: 'host_closed', src: 'Host', msg: 'Host 已关闭' },
  { code: 'route_failed', src: 'Host', msg: '路由失败', note: 'Manifest 声明了 consumes 却无活跃 Consumer 时也用这个码，不降级到 Provider' },
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
    '选下一跳：在 priority ≤ 当前链位上限的活跃实例里取 priority 最大者',
    '每跳把上限降为 consumer.priority − 1，Request 因此按 priority 递减传播',
    '同键出现相同最高 priority：consume_priority_conflict，Host 不按注册顺序静默决胜',
    'Proceed 一次性生效，重复调用返回 flow_already_proceeded',
    '跳数上限 64，越界返回 pipeline_hops_exceeded；绕回已访问实例返回 pipeline_loop',
  ],
};

LA.pluginPos = {
  host: { x: 520, y: 56 },
  agent: { x: 520, y: 190 },
  session: { x: 150, y: 330 },
  'llm-openai': { x: 350, y: 330 },
  'model-context': { x: 560, y: 330 },
  tools: { x: 760, y: 330 },
  'file-tools': { x: 680, y: 470 },
  'shell-tools': { x: 880, y: 470 },
  'sandbox': { x: 760, y: 190 },
};

LA.plugins = [
  { name: 'host', label: 'Host', caps: [], status: '平台内核',
    color: 'var(--plat)', toggleable: false, core: true,
    desc: '薄平台内核。发现插件、启动进程、路由 Frame、维护调用闭环、提供 WebUI 平台壳。',
    provides: [], dependsOn: [], requires: [], priority: null,
    methods: [['host.register', '写入路由表，唯一写入口'],
      ['host.proceed', '续传流水线到下一跳，一次性生效'],
      ['host.plugins', '插件与实例状态分组快照'],
      ['host.ui.state', '推送 UI State'],
      ['host.ui.command', '推送 UI Command'],
      ['host.context.create / bind / close', '执行上下文与 Capability 绑定'],
      ['host.instance.create / close / restart', 'Plugin Instance 生命周期']],
    mounts: [], note: 'Host 实际分派 11 个方法。docs/protocol.md 第 5 节只列了 10 个，漏掉 host.proceed。' },

  { name: 'agent', label: 'agent', caps: ['loop', 'agent'], status: '当前实现',
    color: 'var(--teal)', toggleable: false, core: true,
    desc: 'Turn 编排。创建或复用 Session，读取会话模型，写 Fact，投影上下文，调模型，累计流式增量，执行工具，收口取消与异常。',
    provides: ['loop', 'agent'], dependsOn: ['session', 'llm-openai'],
    requires: ['session', 'llm', 'tools', 'model-context'], priority: null,
    methods: [['loop.turn', '创建 Turn 并串行化 Session'], ['loop.cancel', '取消活动 Turn'],
      ['agent.config.get / set', '默认 scheme 与 max_steps'], ['agent.settings.get / save', 'Settings Schema 读写']],
    mounts: [], note: '只提供 Settings，不渲染聊天正文。' },

  { name: 'session', label: 'session', caps: ['session'], status: '当前实现',
    color: 'var(--green)', toggleable: false, core: true,
    desc: 'Session Fact Log、Workspace Registry、模型上下文投影和多会话 UI。拥有 Workspace 归属与当前全部顶层业务 Component。',
    provides: ['session'], dependsOn: [], requires: [], priority: 1000,
    methods: [['session.create', '创建 Session，保存 model / context_window'],
      ['session.append', '写 Fact 并推送 State'], ['session.derive', '纯投影模型上下文'],
      ['session.query / list / info', '读取历史与模型元数据'],
      ['session.state / dock / select / close / refresh / update', 'UI 会话管理'],
      ['session.messages / facts', '按 before_seq 翻页读旧消息与旧 Fact'],
      ['session.stream.update', '接收流式快照'], ['session.workspace.*', 'Workspace 注册、选择与改名']],
    mounts: [['session-history', 'left'], ['session-chat', 'center'], ['session-trace', 'right'], ['session-dock', 'bottom']],
    note: '历史列表按 Workspace 过滤，多会话标签按 Workspace 恢复；dock 聚合 API usage 与最近上下文快照。Fact Log 落在 ./sessions/<id>.jsonl。Component 是 4 个，plugins/README.md 的表格还停在 3 个。' },

  { name: 'llm-openai', label: 'llm-openai', caps: ['llm'], status: '当前实现',
    color: 'var(--rose)', toggleable: false, core: true,
    desc: '无状态 OpenAI-compatible Completion。流式与 JSON 两种模式，增量以归因事件 llm.chunk 返回。',
    provides: ['llm'], dependsOn: [], requires: [], priority: null,
    methods: [['llm.complete', '按调用方 model 选择流式或 JSON 模式，返回实际 model / context_window'],
      ['llm.chunk (evt)', '归因增量事件'], ['llm.cancel', '按 call_id 取消上游 HTTP 请求'],
      ['llm.config.get / set', '插件配置读写，api_key 回显时掩码'],
      ['llm.models.list / set', '模型目录与默认模型'],
      ['llm.settings.get / save', '插件中心设置：base_url、模型目录、默认模型、api_key（明文落盘）']],
    mounts: [], note: '取消会真的中止上游 HTTP 请求：llm.complete 把 ctx 交给 http.NewRequestWithContext。api_key 明文写在插件目录 config.json，读回时掩码。设置保存后 Host 派发 settings-saved，session-history 刷新模型下拉。' },

  { name: 'model-context', label: 'model-context', caps: ['model-context'], status: '当前实现',
    color: 'var(--violet)', toggleable: true, core: false,
    desc: '透明上下文组装。作为 llm.complete 消费者，把 system prompt 与 runtime context 写成 Session Facts，再 derive、预算控制、快照与确定性压缩。',
    provides: ['model-context'], dependsOn: [], requires: ['session', 'tools'], priority: 2000,
    methods: [['llm.complete (Consumer, prio 2000)', 'prepare 后 Proceed 到 Provider'],
      ['model-context.build', '干跑下一次请求，不写 Fact'], ['model-context.inspect', '按 session 还原最新快照消息'],
      ['model-context.compact', '手工压缩']],
    mounts: [], note: 'hash 变化才写 system / runtime Fact；快照记录 surface 与 truncated_seqs，inspect 用 messages_hash 校验还原。manifest 没有 depends_on，只在运行时弱依赖 session 与 tools。' },

  { name: 'tools', label: 'tools', caps: ['tools'], status: '当前实现',
    color: 'var(--amber)', toggleable: true, core: false,
    desc: '通用 Tool 注册表与分发器。按 (capability, method) 分发到 Provider，不直连 Provider，不拥有 file.* 与 shell.* 路由。',
    provides: ['tools'], dependsOn: [], requires: [], priority: 0,
    methods: [['tools.sync', 'Provider 原子替换来源工具集'], ['tools.register / unregister', '增量注册与注销'],
      ['tools.list', 'Generation 与 active 状态过滤'], ['tools.call', '按 target_capability.target_method 发起 Host Call']],
    mounts: [], note: '注册表缓存到 registry.json。缺少 tools 时 agent 跳过工具阶段并在 request_header.meta.degraded 写入原因，Turn 不失败。' },

  { name: 'file-tools', label: 'file-tools', caps: ['file'], status: '当前实现',
    color: 'var(--amber2)', toggleable: true, core: false,
    desc: 'Workspace 范围内的文件读写八件套，带风险标注。',
    provides: ['file'], dependsOn: ['tools'], requires: ['tools'], priority: 0,
    methods: [['file.read / list / stat / search / glob', 'RiskLow'],
      ['file.write', '原子写入，RiskHigh'], ['file.replace / patch', '字面替换与有序编辑，RiskHigh']],
    mounts: [], note: '工具名 file_read … file_patch，TargetCapability 为 file。' },

  { name: 'shell-tools', label: 'shell-tools', caps: ['shell'], status: '当前实现',
    color: 'var(--amber2)', toggleable: true, core: false,
    desc: '同步命令执行与进程树取消。',
    provides: ['shell'], dependsOn: ['tools'], requires: ['tools'], priority: 0,
    methods: [['shell.execute', '工具名 shell_execute，RiskCritical']],
    mounts: [], note: '取消时终止进程树（Windows 用 Job Object，Unix 用进程组），这是实现细节不是注册方法。交互式终端与后台 Shell 未实现。' },

  { name: 'sandbox', label: 'sandbox', caps: ['sandbox'], status: '当前实现',
    color: 'var(--rose)', toggleable: true, core: false,
    desc: '工具调用链上的权限裁决。只裁决，不执行工具。',
    provides: ['sandbox'], dependsOn: ['tools'], requires: ['session', 'tools'], priority: 500,
    methods: [['tools.call (Consumer, prio 500)', 'allow / modify / deny 三态裁决'],
      ['file.write / replace / patch、shell.execute (Consumer)', '直连高危能力也过裁决'],
      ['sandbox.check', '干跑一次裁决，不写审计'],
      ['sandbox.list_rules / set_rule', '规则集读写，规则只升不降'],
      ['sandbox.settings.get / save', '默认动作与审计保留天数']],
    mounts: [], note: 'autostart 为 false，从插件中心启动后进链。缺席即放行是部署约定，不是平台保证。确认流与确认面板尚未实现。' },
];

LA.edges = [
  { from: 'agent', to: 'session', kind: 'depends' },
  { from: 'agent', to: 'llm-openai', kind: 'depends' },
  { from: 'file-tools', to: 'tools', kind: 'depends' },
  { from: 'shell-tools', to: 'tools', kind: 'depends' },
  { from: 'agent', to: 'tools', kind: 'requires', label: 'tools.list / call' },
  { from: 'agent', to: 'model-context', kind: 'requires', label: '仅声明，不直连' },
  { from: 'model-context', to: 'session', kind: 'requires', label: 'session.derive / append', dip: 132 },
  { from: 'model-context', to: 'llm-openai', kind: 'pipeline', label: 'Proceed · prio 2000' },
  { from: 'sandbox', to: 'tools', kind: 'pipeline', label: 'Proceed · prio 500' },
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
  { id: 'file', label: 'file-tools', sub: 'Provider' },
];

/* needs: 依赖的插件开关；altTo: 未满足时改写目标；skip: 未满足时的说明 */
LA.scenarios = {
  model: {
    label: '模型选择',
    steps: [
      { from: 'center', to: 'bridge', frame: 'http', cap: 'llm', method: 'settings.get', label: '插件中心 → 设置 → llm-openai', note: 'Host Settings Renderer 读取 values，包括 models / default_model。' },
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

LA.turnNote = '取消与异常只影响一个 Session 的活动 Turn。Tool 轮里的 file.* 与 shell.* 由 Provider 执行，tools 只做注册与分发。Turn 结束后的 Dock 用量来自 llm_usage；上下文明细通过 model-context.inspect 还原，下一次预览通过 build，不写 Fact。聊天区按 turns 摘要折叠过程：完成轮默认收起，最终回答与用户消息常显。';

LA.slots = {
  top: { tag: 'Host 控件', owner: 'Host', mount: '—', body: 'v1 的 top 由 Host 控件保留，插件使用其余四个 Slot。内容：插件中心入口（插件 / 设置 / 依赖关系）与 Diagnostic 悬浮窗。' },
  left: { tag: 'session-history', owner: 'session', mount: 'history', body: 'Workspace 分组、会话列表和新会话模型下拉。模型目录在插件中心设置页维护；保存后通过 settings-saved 事件刷新下拉。点击会话发 session.select / session.workspace.select，收到 State 后重绘。' },
  center: { tag: 'session-chat', owner: 'session', mount: 'chat', body: '多会话 Pane。发送调用 loop.turn，取消调用 loop.cancel；流式增量来自 Stream Command，落盘后由 State 重建。消息按 Turn 折叠：完成轮默认收起过程，保留用户消息和最终回答；Turn 头显示状态、过程条数、工具数和时长。' },
  right: { tag: 'session-trace', owner: 'session', mount: 'trace', body: 'Fact Log 与上下文快照视图。业务含义由 session 的 communication.md 定义。' },
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
   不是独立的 Fact 类型。runtime_context 那条在代码里 role 是 user。 */
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
];

LA.nongoals = ['多用户', '远程访问', '跨设备同步', '插件市场', '任意第三方代码沙箱', '调度配额', '在线迁移', '实例状态恢复'];
LA.removed = ['web-tools', 'subagent'];
LA.notImplemented = ['交互式终端', '后台 Shell'];

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
  ['CONSTRAINT.md', '文档与代码的九条硬约束'],
  ['docs/protocol.md', 'Frame、Host 方法、pending 与错误码'],
  ['docs/development.md', '平台模块、结构体和图解'],
  ['docs/pluginsdk.md', '插件开发接口'],
  ['docs/manifest.md', 'Manifest、UI 与 Settings 静态契约'],
  ['docs/webui.md', 'Host WebUI 与 Browser Bridge'],
  ['docs/plugin-dev.md', '插件包结构与开发流程'],
  ['docs/build.md', '构建流程、增量缓存与 dist 布局'],
  ['plugins/<name>/', '该插件的方法、payload、状态与前端交互'],
];

LA.reading = [
  'cmd/liteagent-server/server.go — 组装顺序，UI Publisher 必须在挂载前接好；',
  'protocol/frame.go — Frame 结构、版本与 16 MiB 上限；protocol/errors.go — 30 个稳定错误码；',
  'internal/host/transport.go — routeFrame、pending 与 forwardPipeline；',
  'internal/host/pipeline.go — priority 选链、Proceed 与流水线终态；',
  'internal/host/call.go — Host 发起 Call 的闭环与 Execution Context 绑定；',
  'internal/webui/bridge.go — Browser Call 到 Host Call 的转换；',
  'internal/webui/publisher.go — State 与 Command 的推送信封；',
  'pluginsdk/plugin.go — Call、Emit、Proceed 与取消；',
  'plugins/agent/turn.go — buildModelContext、runTurn 与终态收口；',
  'plugins/session/ui_state.go — Fact 到 State 的重建；',
  'plugins/model-context/builder.go — 上下文 Facts、derive、预算与压缩；',
  'plugins/llm-openai/openai.go — 流式与 JSON 两种上游模式。',
];
