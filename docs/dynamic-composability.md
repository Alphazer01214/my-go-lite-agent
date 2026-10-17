# Cordis 对比与 LiteAgent 插件设计

本文汇总 Cordis 论文对比、动态组合能力和插件体系设计。本文是待实现方案，不描述已完成代码。

实现时应一次完成一个可验收切片，并同步修改 `docs/protocol.md`、`docs/manifest.md` 或 `docs/pluginsdk.md`。

## 1. 文档状态

### 1.1 内容分区

| 分区 | 状态 | 用途 |
|---|---|---|
| 当前实现 | 已存在 | 描述代码现有能力 |
| P0 | 已确认，待实现 | Contract、Provider 生命周期和 Generation |
| P1 | 已确认，待实现 | Go `context.Context` 驱动的 Call 生命周期 |
| Frame Pipeline | 目标设计 | Plugin Priority、`consumes` 和 `Proceed` |
| 插件体系 | 目标设计 | 插件职责和组合关系 |
| 暂不实现 | 已排除 | 当前阶段不进入实现 |

Frame Pipeline 的 Response 返回顺序仍待确认。其他实现细节留到统一实现阶段讨论。

### 1.2 范围

本文面向 Host、pluginsdk、Manifest、Contract Registry 和插件设计者。插件业务 payload 由各插件契约定义。

术语以 [CONTEXT.md](../CONTEXT.md) 为准。现有 Manifest 约束见 [manifest.md](manifest.md)。

## 2. Cordis 论文对比

论文文件为 [Cordis paper](<Cordis paper do not open it unless it is required.md>)。

### 2.1 核心观点

Cordis 把动态组合拆成两个维度：

1. 时间可组合性。组件卸载时，共享环境中的副作用应可撤销。
2. 空间可组合性。组件依赖应可声明、解析和响应式变化。

论文进一步提出：

- Revertible Effect。副作用与逆操作一起返回。
- Reactive Coeffect。依赖满足后激活，依赖撤回时去激活。
- Unified Context。Effect 与 Coeffect 通过同一 Context 协调。
- Fiber Lifecycle。组件实例通过激活、去激活和卸载状态机运行。
- Declarative Loader。配置通过增量调和转换为运行状态。

### 2.2 当前项目已有基础

| Cordis 概念 | 当前对应 |
|---|---|
| Component | Plugin |
| Fiber | Plugin Instance |
| Coeffect Context | Execution Context 的 Capability Binding |
| Provider Identity | Instance ID 与 Generation |
| Declarative Loader | Manifest、Autostart 和 Plugin 生命周期 |
| Contract | `contracts` 中的跨插件 Schema |
| Compensation | Session Fact Log 和取消事实 |

### 2.3 当前项目的优势

1. 每个 Plugin 使用独立进程。插件崩溃不会直接破坏 Host。
2. 进程退出可以回收内存、句柄和 goroutine。
3. 路由只使用 `(capability, method)`。插件不能通过插件名直连。
4. `from` 由 Host 注入，插件不能伪造调用来源。
5. Execution Context 已支持父子范围、参与者和 Instance Binding。
6. Session Fact Log 适合记录外部持久操作，比强行逆写更可靠。
7. 核心插件数量较少，开发和调试成本较低。

### 2.4 当前项目的缺口

1. `provides` 和 `requires` 尚未形成可验证 Contract。
2. 插件进程启动不等于能力注册完成。
3. Provider 停止不会驱动弱依赖或硬依赖进入明确状态。
4. Binding 没有固定 Provider Generation。
5. Call 没有统一 Go `context.Context` 和取消传播。
6. Frame 没有跨插件消费顺序。
7. Config Revision 与运行中 Settings 修改的关系不够清楚。
8. 多 Provider、热替换、权限和沙箱尚未进入当前范围。

### 2.5 采用原则

LiteAgent 采用 Cordis 的语义，不照搬其内存型 `ctx.effect` 实现。

建议边界如下：

1. Host 负责生命周期、Call、Frame Pipeline 和公共 Contract。
2. Plugin 进程继续作为故障边界。
3. 业务持久写入使用幂等、追加日志或补偿。
4. 不建立通用 Effect Ledger 或 Resource Ledger。
5. 不为了形式完整性拆分过多插件。

## 3. P0 动态组合

### 3.1 Capability Contract

一个 Capability 对应一个 Contract。Capability 名就是 Contract 身份，不增加独立 Contract ID。

`provides` 和 `requires` 继续只保存 Capability 名：

```json
{
  "provides": ["llm"],
  "requires": ["session"]
}
```

Contract Registry 集中定义一次：

| 内容 | 用途 |
|---|---|
| Capability 名 | 连接 Manifest 和 Frame 路由 |
| `major` | 标识破坏兼容的公共契约 |
| `required_methods` | Provider 必须注册的最小方法集合 |
| Method Schema | Request、Response 和 Event 的消息引用 |

建议结构：

```json
{
  "llm": {
    "major": 1,
    "required_methods": ["complete", "cancel"],
    "methods": {
      "complete": {
        "request": "#/$defs/LLMCompleteIn",
        "response": "#/$defs/LLMCompleteOut",
        "event": "#/$defs/LLMChunk"
      }
    }
  }
}
```

### 3.2 兼容规则

1. `provides` 中的 Capability 必须有 Contract。
2. `requires` 中的 Capability 必须有 Contract。
3. Provider 必须注册全部 `required_methods`。
4. Provider 的 Contract `major` 必须与 Host 一致。
5. 额外方法不属于稳定契约。

新增可选字段、方法或 Event channel 不增加 `major`。删除、重命名、修改类型、必填性或行为语义时增加 `major`。

`plugin.json.version` 是插件发布版本。`plugin.json.protocol` 是平台协议版本。Contract `major` 只表达 Capability 公共语义。

### 3.3 Provider 生命周期

公共状态如下：

| 状态 | 定义 | 接收新 Call |
|---|---|---|
| `inactive` | 没有进程和运行贡献 | 否 |
| `loading` | 进程启动并完成注册前的阶段 | 否 |
| `active` | 进程存活且必需方法注册完成 | 是 |
| `draining` | 等待旧 Call 结束 | 否 |
| `unloading` | 撤销平台资源并终止进程 | 否 |
| `failed` | 生命周期无法继续 | 否 |

转换关系：

```text
inactive → loading → active
active → draining → unloading → inactive
loading、active、draining、unloading → failed
```

`registering` 属于 `loading` 内部阶段，不进入公共状态。

### 3.4 Provider Record

| 字段 | 含义 |
|---|---|
| `plugin_name` | Manifest 声明的稳定插件身份 |
| `instance_id` | 一次 Plugin Instance 的进程激活身份 |
| `generation` | 每次启动或重启递增的进程代际 |
| `config_revision` | 创建 Instance 时使用的不可变配置快照标识 |
| `desired_state` | `running` 或 `stopped` |
| `state` | 当前生命周期状态 |
| `registrations` | 实际注册并被 Host 接受的 Capability 和 Method |
| `failure_code` | 稳定失败码 |
| `failure_message` | 诊断和管理面使用的失败说明 |
| `drain_deadline` | `draining` 的最迟结束时间 |

`healthy`、`stopping` 和 `stopped` 从状态推导，不再作为独立真源。

`config_revision` 不追踪 Instance 启动后的 Settings 修改。若以后需要记录实时配置变化，应增加独立字段。

### 3.5 Execution Binding

Binding 至少包含：

| 字段 | 含义 |
|---|---|
| `capability` | 需要解析的 Capability |
| `instance_id` | 选中的 Plugin Instance |
| `generation` | 固定的进程代际 |

Provider 重启后，即使 `instance_id` 不变，新旧进程也不能视为同一个 Provider。

### 3.6 P0 验收

1. `provides` 和 `requires` 只保存 Capability 名。
2. 每个声明的 Capability 都能解析到唯一 Contract。
3. Provider 缺少必需 Method 时不能进入 `active`。
4. Contract Major 不匹配时返回稳定错误码。
5. `loading` 状态下不路由业务 Call。
6. Binding 能识别新的 `generation`。
7. 生命周期派生字段不重复存储。
8. Contract、路由和生命周期均有测试。

## 4. P1 统一 Call 生命周期

P1 只实现 Go `context.Context` 驱动的 Call 生命周期。Go `context.Context` 控制一次 Call。Execution Context 继续表示 Host 的调度范围。

### 4.1 接口方向

所有跨进程 Call 都接收 Go `context.Context`：

```go
Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
```

Handler 也接收 Go `context.Context`：

```go
HandleFunc func(ctx context.Context, req *Request) (json.RawMessage, error)
```

Handler 发起下游 Call 时传递同一个 Go `context.Context`。取消和期限沿 Call 图传播。

`ctx` 必须非空。无限期调用显式使用 `context.Background()`。P1 不设置全局默认 deadline。

### 4.2 Call 状态

| 状态 | 定义 |
|---|---|
| `running` | Request 已发送，尚未得到终态 |
| `cancelling` | 已发出取消信号，正在等待终态 |
| `completed` | 收到成功 Response |
| `failed` | 收到错误 Response 或传输失败 |
| `cancelled` | Go `context.Context` 被取消 |
| `deadline_exceeded` | Go `context.Context` 超过期限 |

一个 Call 只有一个终态。取消和 Response 竞态时，首个终态生效。

### 4.3 生命周期

1. Call 记录调用方、目标 Instance 和目标 Generation。
2. Request 携带可选期限和 Call 标识。
3. Handler 收到对应 Go `context.Context`。
4. Context 取消时，SDK 向 Host 发送 `cancel` Frame。
5. Host 转交取消信号，目标 SDK 取消 Handler。
6. Handler 返回后产生唯一终态 Response。
7. Host 清理 pending。
8. Plugin、Instance、Execution Context 和 Host 关闭都触发取消。
9. 目标超过取消宽限期时，Host 合成终态。

### 4.4 Cancel Frame

独立 `cancel` Frame 已确认。Frame 版本升级到 2。

```json
{
  "version": 2,
  "id": "caller-1",
  "type": "cancel",
  "from": "caller-plugin",
  "context_id": "ctx-1",
  "capability": "llm",
  "method": "complete",
  "reason": "cancelled"
}
```

`cancel` Frame 不产生独立 Response。原 Request 仍产生唯一终态 Response。

`reason` 可取：

- `cancelled`
- `deadline_exceeded`
- `context_closed`
- `instance_closed`
- `host_closed`

### 4.5 错误码

| 错误码 | 含义 |
|---|---|
| `call_cancelled` | Call 被取消 |
| `call_deadline_exceeded` | Call 超过期限 |

### 4.6 P1 验收

1. 所有 Call 都要求 Go `context.Context`。
2. 所有 Handler 都能取得 Go `context.Context`。
3. 嵌套 Call 传播同一个 Context。
4. 取消可以到达下游 Handler。
5. deadline 返回 `call_deadline_exceeded`。
6. 普通取消返回 `call_cancelled`。
7. 每个 Call 恰好产生一个终态。
8. 旧 Generation 的取消不影响新 Generation。
9. Call 并发、取消竞态和嵌套传播均有测试。

## 5. Frame Priority 流水线

Frame Pipeline 允许多个 Plugin 按顺序消费同一 `(capability, method)`。Session 可以记录消息，Sandbox 可以检查 Tool Call，Tools 可以在链尾执行。

### 5.1 术语

| 术语 | 含义 |
|---|---|
| Plugin Priority | `plugin.json` 中声明的整数 |
| Consume Declaration | Plugin 对 `(capability, method)` 的消费声明 |
| Frame Priority | 下一个消费者允许使用的优先级上限 |
| Consumer | 处理 Frame 并决定是否继续的 Plugin |
| Proceed | 把 Frame 交给下一个 Consumer |
| Terminal Provider | 链尾产生最终结果的 Plugin |

### 5.2 Manifest

Plugin 自身声明 Priority：

```json
{
  "name": "session",
  "priority": 200,
  "provides": ["session"],
  "consumes": [
    {
      "capability": "tools",
      "method": "call"
    }
  ]
}
```

| 字段 | 含义 |
|---|---|
| `priority` | Plugin 的统一消费顺序 |
| `consumes` | Plugin 参与的 `(capability, method)` |
| `provides` | Plugin 可作为 Terminal Provider 的 Capability |
| `requires` | 弱依赖 Capability |
| `depends_on` | 硬依赖 Plugin 名 |

`priority` 缺省为 `0`。建议使用 32 位有符号整数，初始 Frame Priority 为 `2147483647`。

### 5.3 选择规则

Host 选择满足以下条件的 Consumer：

```text
plugin.priority <= frame.priority
```

然后选择 Plugin Priority 最大者。Consumer 处理后，下一跳设置为：

```text
next.priority = consumer.priority - 1
```

示例：

```text
Frame Priority = 2147483647

session.priority = 200
sandbox.priority = 100
tools.priority = 0
```

调用路径：

```text
session 200
→ sandbox 100
→ tools 0
```

同一 `(capability, method)` 出现相同 Plugin Priority 时返回 `consume_priority_conflict`。Host 不按注册顺序静默决胜。

### 5.4 Proceed

Consumer 通过一次性 `Proceed` 继续流水线：

```go
result, err := req.Proceed(ctx, payload)
```

| 动作 | 结果 |
|---|---|
| 调用 `Proceed` 并返回结果 | 继续并原样返回 |
| 修改 `Proceed` 的结果 | 转换 Response |
| 不调用 `Proceed` 并返回结果 | 当前 Plugin 成为链尾 |
| 不调用 `Proceed` 并返回错误 | 终止 Call |
| 重复调用 `Proceed` | 返回 `flow_already_proceeded` |

Host 管理 Frame Priority。Plugin 不能通过 payload 指定目标插件或跳过高优先级 Consumer。

### 5.5 Request、Response 和 Event

Request 按 Plugin Priority 递减前进。推荐 Response 沿 Request 的返回路径反向传播：

```text
Request:  caller → session → sandbox → tools
Response: tools → sandbox → session → caller
```

这样 Session 最早看到 Request、最晚看到 Response，Sandbox 包裹 Tools。

响应顺序无法同时满足“优先级递减”和“反向返回”。Response 中的 `priority` 只记录流水线位置，不用于重新选择 Consumer。

新建 Event 可以按 Priority 递减传播。属于在途 Call 的流式 Event 沿 Request 返回路径传播。

Response 反向返回仍待最终确认。

### 5.6 Host 与 pluginsdk

Host 路由表从单属主改为 Consumer 列表。每次流水线记录：

| 字段 | 含义 |
|---|---|
| `flow_id` | 流水线标识 |
| `visited` | 已消费 Frame 的 Plugin |
| `return_path` | Request 的返回路径 |
| `frame_priority` | 下一跳优先级上限 |
| `hop_count` | 当前跳数 |

pluginsdk 增加 Request 和 Event 的 `Proceed`。每个消费处理只能调用一次。

### 5.7 风险

1. Plugin 可以把 Priority 设得很大。Host 必须展示 Effective Consumer 和后备顺序。
2. Consumer 不调用 `Proceed` 会终止流水线。
3. Plugin Priority 较粗，同一 Plugin 的所有 Method 使用同一顺序。
4. 流水线必须检测循环并限制最大跳数。
5. 相同 Priority 必须显式冲突，不能依赖安装顺序。

## 6. 插件体系

### 6.1 插件清单

| 插件 | 状态 | 角色 |
|---|---|---|
| `agent` | 现有 | Turn、Step、LLM 和 Tool 编排 |
| `llm-openai` | 现有 | OpenAI-compatible Completion |
| `session` | 现有 | Session Fact Log 和 UI |
| `model-context` | 计划 | Model Context 构建、检查和压缩 |
| `tools` | 计划 | Tool 注册、发现和分发 |
| `skills` | 计划 | Skill 发现、选择和上下文贡献 |
| `mcp` | 计划 | MCP Connector 和 Tool 适配 |
| `memory` | 计划 | 长期记忆检索和整理 |
| `sandbox` | 计划 | 高危 Frame 拦截和检查 |
| `file-tools` | 计划 | 文件读写、列表和搜索 |
| `shell-tools` | 计划 | 命令执行和取消 |
| `web-tools` | 计划 | 搜索和网页读取 |
| `telemetry` | 可选 | 指标、Trace 和性能数据 |
| `browser-tools` | 可选 | 浏览器自动化 |

不增加 `secret-store`。Secret 暂由现有插件配置管理。后续需要统一 Secret 时再单独设计。

### 6.2 命名调整

原构想中的 `context-manager` 改为 `model-context`。Host 已有 Execution Context，两者必须区分。

`file_tools`、`shell_tools` 和 `web_tools` 使用连字符命名：`file-tools`、`shell-tools`、`web-tools`。

### 6.3 三个能力平面

| 平面 | 插件 |
|---|---|
| Agent 编排 | `agent`、`llm-openai`、`model-context`、`session` |
| Context 来源 | `session`、`skills`、`mcp`、`memory`、`tools` |
| Tool 与执行 | `tools`、`file-tools`、`shell-tools`、`web-tools`、`sandbox` |

## 7. Model Context

### 7.1 真源

资源定义与模型执行上下文采用两层真源：

```text
资源定义真源
→ Skills、MCP、Memory 和配置

模型执行上下文真源
→ Session 中的 context_snapshot fact
```

Skill 包、MCP 连接和 Memory Index 不直接存入 Session。每次实际发送给模型的内容写入 `context_snapshot`。

因此 Session 可以准确回答某次 Turn 当时向模型发送了什么。

### 7.2 Snapshot

Snapshot 至少记录：

| 字段 | 含义 |
|---|---|
| `model_context_id` | Model Context 快照标识 |
| `session_id` | 所属 Session |
| `turn_id` | 所属 Turn |
| `revision` | 构建版本 |
| `messages` | 最终模型消息 |
| `tools` | 最终 Tool Definition |
| `sections` | System、Memory、Skill 和 Conversation 等分区 |
| `source_refs` | Session Seq、Skill ID 和 MCP Tool ID |
| `token_estimate` | Token 估算 |
| `budget` | 输入预算和输出预留 |
| `hash` | 快照摘要 |
| `warnings` | 截断、排除和降级信息 |

`model_context_id` 不等于 Host 的 `context_id`。

### 7.3 Context Block

各来源统一转换为 Context Block。Block 包含来源、内容、Token 估算、哈希、是否必需和内部顺序。

Plugin Priority 只决定 Frame 顺序。Context Block 的顺序由 `model-context` 决定，两者不复用。

### 7.4 构建与压缩

构建流程：

```text
读取 Session
→ 获取 Tool、Skill、MCP 和 Memory Contribution
→ 组装 System Prompt
→ 去重
→ 应用 Token Budget
→ 可选压缩
→ 写入 context_snapshot
→ 返回给 Agent
```

压缩不改写 Session Fact Log。压缩结果追加为 `context_compaction` fact，并记录来源 Seq、摘要、策略和 Token 变化。

主要能力使用 `model-context.build`、`model-context.inspect` 和 `model-context.compact`。高效上下文是 `build` 内部策略，不增加独立能力。

## 8. Tools 与 Tool Provider

### 8.1 Tools

`tools` 是 Tool 领域的注册与分发插件，不是通用 Service Broker。

主要职责：

1. 接收 Tool Provider 注册。
2. 返回聚合后的 Tool Definition。
3. 根据 Tool 名称调用目标 Capability。
4. Provider 退出时移除 Tool。
5. 标记 Tool 风险和执行属性。

主要能力为 `tools.list`、`tools.call`、`tools.register` 和 `tools.unregister`。`agent` 只使用 `tools.list` 和 `tools.call`。

Tool Spec 面向模型保存名称、说明和输入 Schema。目标 Capability 和 Method 只供 Host 分发，不发送给模型。

`tools` 不硬依赖具体 Tool Provider。`file-tools`、`shell-tools` 和 `web-tools` 通过弱依赖注册 Tool，避免依赖环。

### 8.2 file-tools

计划能力包括 `file.read`、`file.write`、`file.list`、`file.stat` 和 `file.search`。路径使用 Workspace 相对路径。越界检查由 Sandbox 处理。

### 8.3 shell-tools

计划能力包括 `shell.execute`、`shell.cancel` 和 `shell.status`。执行使用 P1 的 Go `context.Context`。Context 取消时终止子进程。

### 8.4 web-tools

计划能力包括 `web.search`、`web.fetch` 和 `web.cancel`。搜索 Provider 可以后续细分，当前保持统一 `web` Capability。

## 9. Sandbox

Sandbox 是 Frame Consumer，不是 Tool Provider。

它至少消费：

| `(capability, method)` | 用途 |
|---|---|
| `tools.call` | 检查模型 Tool Call |
| `file.write` | 检查文件修改 |
| `shell.execute` | 检查命令执行 |
| `web.fetch` | 检查网络读取 |
| `web.search` | 可选检查搜索内容 |

仅消费 `tools.call` 不够。其他插件可以直接调用 `shell.execute` 并绕过 Tool 层。

Sandbox 可以允许、修改、拒绝或把操作转为待确认状态。它不负责实际执行文件、Shell 和网络操作。

## 10. Context 来源插件

### 10.1 skills

Skills 提供指令、工具规则、参考资料和工作流。计划能力包括 `skills.list`、`skills.get`、`skills.select` 和 `skills.contributions`。

Skill 的可执行部分通过 Tool 执行。Skills 插件不直接执行任意代码。

### 10.2 mcp

MCP Plugin 管理 Connector 生命周期并适配 MCP Tool、Resource 和 Prompt。MCP Tool 注册到 `tools` 时使用命名空间：

```text
mcp.<server>.<tool>
```

`model-context` 只接收标准化 Tool Spec 和 Context Block，不直接理解 MCP 协议。

### 10.3 memory

Memory 管理长期记忆检索、添加、整理和删除。`model-context.build` 只接收检索结果，不接收整个 Memory Store。

Memory Candidate 的来源事实仍写入 Session。这样不会形成第二套会话真源。

## 11. Priority 建议

推荐值只是开发约定，不做硬限制：

| Plugin | Priority | 原因 |
|---|---:|---|
| `telemetry` | `2000` | 最先观察 Frame |
| `session` | `1000` | 记录 Request 和 Response |
| `sandbox` | `500` | 在 Terminal Provider 前检查 |
| 转换类 Plugin | `100` | 参数和结果转换 |
| `tools` 和 Tool Provider | `0` | Terminal Provider |
| 后备 Plugin | 负数 | 低优先级后备 |

同一 `(capability, method)` 的 Priority 必须唯一。

## 12. 依赖关系

```text
agent
├─ depends_on model-context
├─ depends_on llm-openai
└─ depends_on tools

model-context
├─ depends_on session
└─ requires skills、mcp、memory、tools

tools
└─ requires 动态 Tool Provider

file-tools、shell-tools、web-tools
└─ requires tools
```

`tools` 不 `depends_on` 具体 Tool Provider。Sandbox 通过 `consumes` 参与高危 Method，不建立硬依赖环。

## 13. 暂不实现

1. Effect Ledger 和 Resource Ledger。
2. 配置热替换、滚动替换和配置 reconciliation。
3. Capability Grant、权限系统和外部安全沙箱。
4. Service Broker、多 Provider 和负载均衡。
5. `secret-store`。
6. Contract 版本范围解析。
7. Method 级依赖声明。
8. 插件市场和跨主机 Provider。
9. 在线迁移运行中 Call。
10. 业务副作用的通用数学逆操作。
11. 自动推导未声明的 Capability 依赖。
12. 为微小交互强制拆分插件。

现有进程隔离继续保留。它用于故障隔离，不扩展为安全沙箱。

## 14. 实施顺序

| 顺序 | 内容 | 分期 |
|---|---|---|
| 1 | Capability Contract Registry | P0 |
| 2 | Provider 生命周期和注册完成语义 | P0 |
| 3 | Binding Generation | P0 |
| 4 | Context Call 和 Cancel Frame | P1 |
| 5 | Plugin Priority、`consumes` 和 `Proceed` | Frame Pipeline |
| 6 | `tools` 与 Tool Provider | 插件体系 |
| 7 | `model-context` 与 `context_snapshot` | 插件体系 |
| 8 | `skills`、`mcp`、`memory` | 插件体系 |
| 9 | `sandbox` | 插件体系 |

实现时一次只完成一个可验收切片。每个切片同步更新协议、Manifest 或 pluginsdk 文档。

## 15. 待确认决策

1. Response 是否采用 Request 路径反向返回。当前建议采用反向返回。
2. Frame Pipeline 的最大跳数和循环错误码。
3. `context_snapshot` 与 `context_compaction` 的公共 Contract 归属。
4. `tools.register` 是否需要 Tool Definition 版本。

这些决策只修正字段、错误码和验收条件，不重新引入已排除方案。
