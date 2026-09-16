# my-go-lite-agent

轻量 Go agent：一切皆插件，插件运行时发现与组装，核心零第三方（解析库除外）。

## Language

**Plugin**:
一个可独立分发的扩展单元，由配置文件、可执行二进制与静态文件组成；运行时被发现并挂载，通过结构化消息与宿主通信。可只实现 Function，或兼有 Presentation。
_Avoid_: 扩展、模块、组件（除非特指 Go module）

**Host**:
插件树的宿主进程：Discovery、Assembly、生命周期、Frame 路由、Session 不变量与 `agent.request`/`agent.inject`。不实现 Agent Loop；装配/插件错误可见但进程继续。
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
插件的另一条面：向 Render Medium 暴露的可呈现状态与交互意图。主窗口内容分为 markdown_text、message_text、summary_text 三类渲染意图；与 Function 可同属一个 Plugin，也可只实现其一。
_Avoid_: 视图、前端组件、渲染器（规范名词是 Presentation）

**Presentation Card**:
Presentation 面的结构化渲染意图：从 args/result 纯函数投影（如问卷、diff 卡）。不做 I/O，回放可重现。与瞬态 stream/status 信号不同。
_Avoid_: UI 组件、视图模型

**Layout**:
Web Medium 的页面与槽位声明真源：磁盘 layout.json（项目基座）与插件 ui.pages/ui.slots 加法贡献的合并结果。定义 page 列表、slot 几何与 role，不定义组件实现。
_Avoid_: 网格配置、shell 布局、layout 插件

**Slot Role**:
Panel 槽位的语义标签（如 session-view）：说明该槽期望什么类内容。preferred component 可选；同 role 可竞争，由 Assembly 裁决。
_Avoid_: 组件类型约束、slot kind

**Platform Module**:
Web Medium 暴露给 Panel Component 的作者 SDK 面（LiteAgent 全局与 /app/ 下 md/facts 等）。与 Design Token 同级的平台契约，插件可依赖也可自带实现。
_Avoid_: 公共库、shared utils、Shell 内部模块

**Current Session**:
session Capability 上的媒介无关「当前会话」状态：current/select/list/create。id 恒为非空；空值统一映射为 `default`。create 默认铸新 id 并选中，但 `origin=subagent` 的创建不抢占 Current。Web Medium 不再自持专用真源。
_Avoid_: 默认会话、活动会话 id（口语可用）

**Panel**:
Web Render Medium 中一块可被插件填充的 UI 槽位，槽位集合由 Layout 定义。插件经 Panel Component 向 Panel 提供内容：静态挂载由 Manifest 声明，运行时变化经 PanelOp。Assembly 可禁用/覆盖 mount。
_Avoid_: 页面、视图区、slot（可作别名，规范名词是 Panel）

**Panel Component**:
插件作者以原生 Web Component 实现的 Panel 内容单元：自定义元素 + Shadow DOM，经 props 接收数据、经 UI Action 与 SDK 回传交互。元素名以插件名为前缀。
_Avoid_: 组件树、widget、UI 插件

**Session View**:
Session Log 的主视图组件：重放历史事实、实时消费 Presentation 信号、提供输入入口以触发 Turn。由 session 插件经 Panel 提供；替代视图可经 Assembly 声明挂载同一槽位与之竞争。
_Avoid_: 聊天窗口、chat 插件（"聊天"仅作口语别名）、聊天面

**Trace**:
Session Log 的调试投影：一次会话的全部事实按发生序呈现。不是独立的记录或审计流——数据与 Session View 同源（Session Log），只是投影粒度不同。
_Avoid_: 日志、监控、审计日志

**UI Entry**:
Manifest `ui.entry` 指向的插件 ES Module：加载时注册该插件的全部 Panel Component，由 Shell 经 /plugin-ui/ 动态 import。
_Avoid_: 入口页、index.html（旧 HTML 注入形态）

**PanelOp**:
对 Panel 的一次变更指令：`set`（按 id 重挂载组件并设 props）或 `clear`（按 id 移除）。插件经 Frame 发出，Host 校验后向 Render Medium 广播。
_Avoid_: 注入指令、panel 事件

**Design Token**:
Shell 暴露给 Panel Component 的 `--la-*` CSS 自定义属性集合：组件样式与宿主主题联动的唯一契约。
_Avoid_: 主题变量、CSS 变量（泛称可用，规范名词是 Design Token）

**UI Action**:
Panel 内控件触发的回传事件：Host 将其包成 `cap=ui, method=action` 的 req 发给目标插件。与 Command 同属交互入口，但绑定在组件树节点上而非 `/` 前缀。
_Avoid_: 回调、事件总线消息（规范名词是 UI Action）

**Command**:
主窗口内以 `/` 触发的交互入口。原生命令由 Host 内建；插件经 Manifest 的 commands 声明附加命令。与原生命令同名的插件不予加载。
_Avoid_: 指令、slash command（可作别名，规范名词是 Command）

**Manifest**:
插件目录中 plugin.json 声明的元数据：名称、版本、协议、provides/consumes、入口、说明与命令表。Discovery 读入 Host 内存，可经 /refresh 重扫更新；不是运行参数配置。
_Avoid_: 清单、plugin config（config 是运行参数，不是 Manifest）

**Render Medium**:
消费 Presentation 信号并向用户展示的媒介。内建实现有二：CLI Medium（liteagent-cli）与 Web Medium（liteagent-server），共享同一内核与插件协议，分歧仅在前端。契约按可多消费者订阅设计。
_Avoid_: 前端、UI 进程、renderer（规范名词是 Render Medium）

**Shell**:
Web Render Medium 中项目提供的最薄骨架：按合并 Layout 渲染 chrome 与导航、提供 Design Token 与必要全局脚本（SDK 与组件装载器）。内容面（聊天、trace 等）不属骨架，由插件作者实现为 Panel Component 注册进页面，项目自有实现也不例外。
_Avoid_: 前端框架、主界面、聊天壳（规范名词是 Shell）

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
一个 Plugin：消费 llm、session、tool、system-prompt、context 等 Capability，提供 `loop` 并执行 Agent Loop。Host 不内建 Agent；无 Agent 插件时不能跑 Turn（装配软失败，使用时报错）。一个 Agent 实例终身绑定一个 Session；Host 可托管多个 Agent 实例。
_Avoid_: 智能体实例、机器人、bot、agent 组装（规范名词是 Agent）

**Agent Loop**:
驱动 Turn/Step 的执行策略：组装 Model Context、调用 LLM、调度工具、写回 Session Log 边界事实（turn/step/request_header/llm_usage）。由提供 `loop` 的 Agent 插件实现；Host 只做路由、锁/Cancel/状态与 `agent.request`/`agent.inject` 横切面。
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
由父 Agent 经 tool call 触发的独立 Agent 实例：拥有自己的 Session 与 Agent Loop，产出以 tool result 回传父 Agent。子 Session 元数据记录 `parentSession` / `origin=subagent` / `delegationDepth`；父 Session 可经 list/family 进入查看子 Session，子不抢占 Current Session。`mode=async` 时立即返回子 Session id，完成后经 `agent.inject` 回注结果。
_Avoid_: 子智能体、nested agent、child agent（规范名词是 Subagent）

**Context Manager**:
观测与管理模型上下文状态的插件：拼装 System Prompt，经 `context` Capability 提供 prepare / compact / usage / listContext（查看进入模型的 messages）。tools/skills 说明注入亦归此。历史真源仍是 Session Log，CM 不改写旧事实。
_Avoid_: 上下文管理器、prompt engine、prompt builder（规范名词是 Context Manager）

**Prompt Segment**:
可注册的 System Prompt 片段：带 name/order/text，由 Assembly 静态提供或插件运行时注册。Skills 目录等说明性内容也可作为段注册。
_Avoid_: prompt 段、提示片段、section（可作别名，规范名词是 Prompt Segment）

**Context Prepare**:
Loop 在 `llm.complete` 前拉取的一次上下文产物：最终 messages、tools schema、System Text、usage 占位与 compact 提示。内容选择结果必须能从 Session Log 重建。
_Avoid_: 上下文组装结果、prompt 包（规范名词是 Context Prepare）

**Context Summary**:
Compact 写入 Session Log 的摘要事实（model-visible）：meta 记录覆盖范围（如 coversThroughSeq）；derive 只投影 active summary 及其之后的原文。不删除、不改写旧事实。
_Avoid_: 压缩块、记忆摘要（规范名词是 Context Summary）

**Context Usage**:
单次模型请求的 token 占用观测：优先供应商 usage，缺失时字符估算。经 log/接口暴露给 CLI 与 Render Medium，不是第二真源。
_Avoid_: 配额、计费用量（若语义不同）

**Context Window**:
模型单次请求可容纳的最大上下文长度（token）。由 LLM 插件声明、用户可覆盖；仅作观测与 soft 阈值比较，不是 Session 真源。
_Avoid_: 窗口大小、max_tokens（采样参数，语义不同）

**Tool Result Stub**:
derive 对超出全文保留窗口的历史 `tool_result` 的短投影（说明截断与如何恢复）。Session Log 原文不变；`tool_call` 永不 stub。
_Avoid_: 工具结果截断（可作口语）、placeholder

**Config Capability**:
插件运行时配置面：get / set / schema / reload。与 Manifest（元数据）分离；`/refresh` 可广播 reload，不热插拔进程。
_Avoid_: 插件配置文件（磁盘 config.json 只是持久化）、Manifest 参数

**Workspace**:
一次运行中项目文件与工具操作的根目录。CLI 默认启动时 cwd；Web 由会话选择。作为 Session 元数据携带，filetools / shelltools / sandbox / skill 等只认此根，不认全局 cwd。
_Avoid_: 工作目录、项目路径、repo root（口语可用，规范名词是 Workspace）

**Permission**:
对一次工具调用的 allow / ask / deny 裁决及规则集合。由 provides `policy` 的插件持有规则；Agent Loop 在执行工具前查询。不是路由中间件。
_Avoid_: 审批流、ACL、Interceptor（已废弃）、权限系统（过泛）

**Policy Capability**:
Permission 的可调用面：decide（对一次 tool call 给出裁决）与必要的规则观测。消费方是 Agent Loop；Host 不内建策略引擎。
_Avoid_: 审批 Capability、guard、gate

**Read-only Tool**:
工具 schema 上的只读标志（`readOnly`）：声明该工具不修改 Workspace 外状态。并行调度与默认策略可依赖该标志；与 Permission 规则正交。
_Avoid_: 安全工具、只读能力（泛称）

**Skill**:
工作区内可被模型或用户触发装载的一份说明性能力包（约定目录下的文档）。目录名进 System Prompt；全文在触发时注入。
_Avoid_: 技能文件、prompt pack（可作别名）、插件技能（Skill 不是 Plugin）

**Skill Manager**:
发现与装载 Skill 的插件：扫描 Workspace 约定目录、向 Context Manager 注册目录段、提供模型可调用的 load 工具，并在用户输入阶段展开 `$skill` 触发。不改写 Session Log 旧事实。
_Avoid_: 技能加载器、skills 插件（规范名词是 Skill Manager）

**Skill Trigger**:
用户输入中以 `$` + Skill 名书写的显式触发记号（如 `$review`）。在 Turn 开始、进入模型之前由 Agent 展开为 Skill 全文注入；与模型主动 load 工具并存。
_Avoid_: 斜杠技能、宏、快捷指令

**Todo**:
Turn 内可见的工作计划事实：经工具写入 Session Log，由 Context Prepare 投影进 Model Context。不是独立第二真源。
_Avoid_: 任务列表 UI、待办存储、task tracker

**Plan Constraint**:
对 Agent 在「先计划再动手」阶段的行为约束：仅由 System Prompt 与 Todo 可见性约定，不裁剪 tools 列表、不切换工具门禁。有意相对完整 Plan Mode 的 lite 取舍。
_Avoid_: Plan Mode（若指工具子集门禁）、只读模式（语义不同）
