# my-go-lite-agent

轻量 Go agent：一切皆插件，插件运行时发现与组装，核心零第三方（解析库除外）。

## Language

**Plugin**:
一个可独立分发的扩展单元，由配置文件、可执行二进制与静态文件组成；运行时被发现并挂载，通过结构化消息与宿主通信。可只实现 Function，或兼有 Presentation。
_Avoid_: 扩展、模块、组件（除非特指 Go module）

**Host**:
插件树的宿主进程：负责 Discovery、Assembly、生命周期、Frame 路由与双层 Waterfall 的薄内核。
_Avoid_: 宿主程序、主程序、kernel（正文可用，规范名词是 Host）

**Assembly**:
一次运行时对插件树的组装结果：哪些插件被挂载、以何序、何参数。发现 ≠ 挂载。
_Avoid_: 配置加载（配置只是 Assembly 的输入之一）

**Discovery**:
扫描插件目录与清单，得到「机器上有哪些插件」的过程。只负责看见，不负责挂载。
_Avoid_: 加载、注册（挂载属于 Assembly）

**Capability**:
宿主或插件对外提供的一项可调用能力，经 Frame 暴露；消费方只依赖契约，不依赖具体插件。
_Avoid_: 功能、接口实现

**Function**:
插件的一条面：接收 Call Payload、处理，并返回 result / additionalContexts，或经 Host 请求其他 Capability。一次调用，不编排下一步。
_Avoid_: 业务逻辑、实现体（规范名词是 Function）

**Presentation**:
插件的另一条面：向 Render Medium 暴露的可呈现状态与交互意图。主窗口内容分为 markdown、expandable、message 三类渲染意图；与 Function 可同属一个 Plugin，也可只实现其一。
_Avoid_: 视图、前端组件、渲染器（规范名词是 Presentation）

**Presentation Card**:
Presentation 面的结构化渲染意图：从 args/result 纯函数投影（如问卷、diff 卡）。不做 I/O，回放可重现。与瞬态 stream/status 信号不同。
_Avoid_: UI 组件、视图模型

**Render Medium**:
消费 Presentation 信号并向用户展示的媒介。Host 内建 CLI 是默认实现；契约按可多消费者订阅设计。
_Avoid_: 前端、UI 进程、renderer（规范名词是 Render Medium）

**Waterfall**:
环绕式拦截链：下游处理完才返回，监听方不放行则短路。Host 内建强制链 + 可选外部 Interceptor。
_Avoid_: 中间件链、拦截器（若语义相同，正文可用，规范名词是 Waterfall）

**Interceptor**:
挂在 Waterfall 上的策略插件：放行、改写或短路一次调用。
_Avoid_: 中间件、守卫（守卫若语义为单调否决可另立术语）

**Frame**:
Host 与插件之间的一条完整结构化消息：`uint32` 长度前缀 + JSON body，携带 id 以对齐请求/响应。
_Avoid_: 包、报文（规范名词是 Frame）

**Session Log**:
仅追加的会话事实流；是交互历史的唯一真源。模型历史从日志派生，从不单独存储。
_Avoid_: 对话历史、transcript（可作别名，规范名词是 Session Log）

**Model Context**:
单次模型请求实际可见的输入集合；必须能从 Session Log 重建。
_Avoid_: 上下文、prompt（prompt 是 Model Context 的组装产物之一）

**Call Payload**:
Host 调某个 Capability 时的输入（tool args、messages…）。不是 Model Context，也不是 Session Log。
_Avoid_: 参数、请求体（规范名词是 Call Payload）

**Additional Contexts**:
Function 返回结果时附带的、在工具结果之后注入的模型可见消息。由 Loop 落入 Session Log，不是插件直接改写历史。
_Avoid_: 附加上下文、注入内容（规范名词是 Additional Contexts）

**Agent**:
一个 Plugin：消费 llm、session、tool 等 Capability 并执行 Agent Loop。Host 可内建默认 Agent，外部 Agent 插件经 loop Capability 替换。一个 Agent 实例终身绑定一个 Session；Host 可托管多个 Agent 实例。
_Avoid_: 智能体实例、机器人、bot、agent 组装（规范名词是 Agent）

**Agent Loop**:
驱动 Turn/Step 的执行策略：组装 Model Context、调用 LLM、调度工具、写回 Session Log。默认编译在 Host，可由 loop Capability 替换。
_Avoid_: 主循环、orchestrator、执行器（规范名词是 Agent Loop，简称 Loop）

**Turn**:
Agent 对一批待处理输入的完整响应周期：由零个或多个 Step 组成，欠账清零后关闭。
_Avoid_: 轮次（口语可用，规范名词是 Turn）、conversation round

**Step**:
一次模型请求 + 该响应引发的工具执行。Turn 内的最小完整执行单元。
_Avoid_: 步骤、iteration、round（规范名词是 Step）

**System Prompt**:
以 system 角色进入模型的指令内容。作为 Session Log 的模型可见事实可被重建，不是旁路通道。
_Avoid_: 系统提示、system message（可作别名，规范名词是 System Prompt）

**Request Header**:
一次模型调用的配置快照（provider、model、采样参数等），落入 Session Log 以便审计与重放。
_Avoid_: 请求头、调用配置（规范名词是 Request Header）

**Subagent**:
由父 Agent 经 tool call 触发的独立 Agent 实例：拥有自己的 Session 与 Agent Loop，产出以 tool result 回传父 Agent。
_Avoid_: 子智能体、nested agent、child agent（规范名词是 Subagent）

**Context Manager**:
提供 system-prompt Capability 的插件：持有 Prompt Segment 注册表，按序拼装 System Prompt。v1 只做组装，不做历史压缩。
_Avoid_: 上下文管理器、prompt engine、prompt builder（规范名词是 Context Manager）

**Prompt Segment**:
可注册的 System Prompt 片段：带 name/order/text，由 Assembly 静态提供或插件运行时注册。
_Avoid_: prompt 段、提示片段、section（可作别名，规范名词是 Prompt Segment）
