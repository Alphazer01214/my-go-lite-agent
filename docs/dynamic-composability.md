# Cordis 对比与 LiteAgent 插件设计

本文汇总 Cordis 论文对比、动态组合能力和插件体系设计。本文是下一轮实现的统一设计输入。

设计阶段已经结束。实现时一次完成一个可验收切片，并同步修改 `docs/protocol.md`、`docs/manifest.md` 或 `docs/pluginsdk.md`。

## 1. 设计边界

### 1.1 状态

| 内容 | 状态 |
|---|---|
| 当前项目 | 以代码和现有文档为准 |
| Provider 生命周期 | 已实现 |
| Generation | 已实现 |
| Context Call | 已实现 |
| Cancel Frame | 已实现 |
| Frame Priority 流水线 | 已实现 |
| 新插件 | 待实现 |

### 1.2 概念预算

平台只新增五个概念：

| 概念 | 作用 |
|---|---|
| Generation | 标识当前插件进程 |
| Priority | 决定插件消费 Frame 的顺序 |
| Consumer | 参与处理 Frame 的插件 |
| Proceed | 把 Frame 交给下一个插件 |
| Model Context Snapshot | 记录模型实际输入 |

不引入 Fiber、Effect、Coeffect、Realm、Lease、Ledger、Broker 等概念。必要算法留在模块内部。

### 1.3 术语

现有术语见 [CONTEXT.md](../CONTEXT.md)。Manifest 约束见 [manifest.md](manifest.md)。

新增词使用以下固定含义：

- `model-context` 管理 Model Context，不管理 Execution Context。
- Consumer 只表示参与 Frame 流水线的插件。
- Proceed 只表示继续下一跳。
- Generation 只表示进程启动代际。

## 2. Cordis 对比

论文文件为 [Cordis paper](<Cordis paper do not open it unless it is required.md>)。

### 2.1 值得采用的语义

Cordis 将动态组合拆成时间与空间两个维度：

1. 插件离开时，其运行贡献应收敛。
2. 插件依赖应可声明、解析和变化。

LiteAgent 采用以下思想：

- 插件进程启动完成后才能提供能力。
- Provider 更换时使用新的 Generation。
- Call 使用统一 Context 取消。
- 多个插件可以按顺序消费同一 Frame。
- Model Context 保存可追踪快照。

### 2.2 不采用的模型

不照搬 Cordis 的内存型 Effect、Coeffect 和 Fiber 实现。

原因如下：

1. LiteAgent 已使用进程隔离作为插件边界。
2. 通用逆操作无法覆盖网络和持久写入。
3. 完整 Effect Ledger 会扩大 Host 与插件接口。
4. 当前插件数量较少，不需要通用 Service Broker。
5. 细粒度模型会增加插件开发成本。

### 2.3 当前项目优势

1. 每个插件运行在独立进程中。
2. 插件崩溃不会直接破坏 Host。
3. Frame 路由只使用 `(capability, method)`。
4. Host 注入 `from`，插件不能伪造来源。
5. Execution Context 已支持父子范围和 Instance Binding。
6. Session Fact Log 适合保存可追踪的业务事实。
7. 核心模块和插件边界清楚。

### 2.4 当前缺口

1. Model Context 尚无统一快照。

## 3. Provider 生命周期

### 3.1 公共状态

只公开五个状态：

| 状态 | 定义 | 接收新 Call |
|---|---|---|
| `inactive` | 没有进程和运行贡献 | 否 |
| `loading` | 进程启动并完成注册前的阶段 | 否 |
| `active` | 进程存活且注册完成 | 是 |
| `stopping` | 不接收新 Call，等待旧 Call 并清理 | 否 |
| `failed` | 生命周期无法继续 | 否 |

`draining` 和 `unloading` 是 `stopping` 的内部阶段。它们可以出现在日志中，但不进入公共状态。

### 3.2 转换

```text
inactive → loading → active
active → stopping → inactive
loading、active、stopping → failed
```

Provider 只有在进程存活且 `host.register` 完成后才能进入 `active`。

### 3.3 Provider 信息

| 字段 | 含义 |
|---|---|
| `plugin_name` | Manifest 声明的插件身份 |
| `instance_id` | 一次进程激活身份 |
| `generation` | 每次启动或重启递增的进程代际 |
| `config_revision` | 创建 Instance 时使用的配置快照标识 |
| `state` | 公共生命周期状态 |
| `registrations` | 实际注册的 Capability 和 Method |
| `failure_code` | 稳定失败码 |
| `failure_message` | 失败说明 |

不保存 `desired_state`、`healthy`、`stopping`、`stopped` 或 `drain_deadline`。

Start 和 Stop 是命令，不建立第二套期望状态。

### 3.4 Generation

每次启动或重启都会增加 Generation。Binding 至少保存：

| 字段 | 含义 |
|---|---|
| `capability` | 需要解析的 Capability |
| `instance_id` | 选中的 Plugin Instance |
| `generation` | 固定的进程代际 |

Provider 重启后，即使 `instance_id` 不变，新旧进程也不是同一个 Provider。

Host 创建或修改 Binding 时自动捕获当前 Generation。调用方只传 `instance_id`。Generation 不一致时，调用返回 `context_binding_invalid`。

### 3.5 Config Revision

`config_revision` 是创建 Plugin Instance 时使用的不可变配置快照标识。它不追踪启动后的 Settings 修改。

如果以后需要记录实时配置变化，应增加独立字段。当前不扩展 Config Revision 的语义。

### 3.6 验收

1. `loading` 状态不接收业务 Call。
2. `host.register` 完成后才进入 `active`。
3. `stopping` 不接收新 Call，旧 Call 可以继续。
4. Binding 能识别新的 Generation。
5. 生命周期字段没有重复真源。
6. 启动、重启、停止和崩溃均有测试。

Provider 生命周期与 Binding Generation 已完成本节全部验收。

## 4. Context Call

Go `context.Context` 控制一次 Call。Execution Context 继续表示 Host 的调度范围。两者不是同一概念。

### 4.1 接口

所有 Call 都接收 Go `context.Context`：

```go
Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
```

所有 Handler 都接收 Go `context.Context`：

```go
HandleFunc func(ctx context.Context, req *Request) (json.RawMessage, error)
```

Handler 发起下游 Call 时传递同一个 Go `context.Context`。取消和期限沿 Call 图传播。

`ctx` 必须非空。无限期调用显式使用 `context.Background()`。不设置全局默认 deadline。

### 4.2 Call 状态

只保留：

| 状态 | 定义 |
|---|---|
| `running` | Call 尚未结束 |
| `cancelling` | 已取消，正在等待终态 |
| `completed` | 成功完成 |
| `failed` | 错误或传输失败 |
| `cancelled` | Context 被取消 |

deadline 是失败原因。它对应 `call_deadline_exceeded`，不增加独立生命周期。

一个 Call 只有一个终态。取消和 Response 竞态时，首个终态生效。

### 4.3 取消

Context 取消后：

1. SDK 发送 `cancel` Frame。
2. Host 转交目标 SDK。
3. 目标 SDK 取消 Handler Context。
4. Handler 返回唯一终态 Response。
5. Host 清理 pending。
6. 超过取消宽限期时，Host 合成终态。

Plugin、Instance、Execution Context 和 Host 关闭都触发取消。

### 4.4 Cancel Frame

`cancel` 使用独立 Frame 类型。Frame 版本升级到 2。

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

`cancel` 不产生独立 Response。原 Request 仍产生唯一终态 Response。

| 字段 | 含义 |
|---|---|
| `id` | 被取消的 Call |
| `type` | 固定为 `cancel` |
| `from` | Host 注入的真实调用方 |
| `context_id` | 所属 Execution Context |
| `capability` | 原 Call 的 Capability |
| `method` | 原 Call 的 Method |
| `reason` | 取消或超时原因 |

### 4.5 错误码

| 错误码 | 含义 |
|---|---|
| `call_cancelled` | Call 被取消 |
| `call_deadline_exceeded` | Call 超过期限 |

### 4.6 验收

1. 所有 Call 和 Handler 都使用 Go `context.Context`。
2. 嵌套 Call 传播同一 Context。
3. 取消可以到达下游 Handler。
4. deadline 返回稳定错误码。
5. 每个 Call 恰好产生一个终态。
6. 旧 Generation 的取消不影响新 Generation。
7. Call 并发、取消和嵌套传播均有测试。

Context Call 与 Cancel Frame 已完成本节全部验收。

## 5. Frame Priority 流水线

Frame Priority 允许多个插件按顺序处理同一 `(capability, method)`。Session 可以记录消息，Sandbox 可以检查 Tool Call，Tools 可以在最后执行。

### 5.1 Manifest

Plugin 在 `plugin.json` 中声明 Priority：

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
| `priority` | 插件消费 Frame 的顺序 |
| `consumes` | 插件参与的 `(capability, method)` |
| `provides` | 插件可以提供的 Capability |
| `requires` | 弱依赖 Capability |
| `depends_on` | 硬依赖插件名 |

`priority` 是 32 位有符号整数，缺省为 `0`。数值越大，越先处理。

### 5.2 Frame Priority

Request 和 Event 增加 `priority` 字段。Host 将新流水线的初始值设为：

```text
2147483647
```

该字段由 Host 管理。Plugin 不能通过 payload 指定下一跳或跳过高优先级插件。

### 5.3 顺序

Host 选择满足以下条件的 Consumer：

```text
plugin.priority <= frame.priority
```

然后选择 Priority 最大者。Consumer 处理后，下一跳为：

```text
next.priority = consumer.priority - 1
```

示例：

```text
session 200
→ sandbox 100
→ tools 0
```

同一 `(capability, method)` 出现相同 Priority 时返回 `consume_priority_conflict`。Host 不按注册顺序静默决胜。

### 5.4 Proceed

Consumer 通过一次性 `Proceed` 继续：

```go
result, err := req.Proceed(ctx, payload)
```

| 动作 | 结果 |
|---|---|
| 调用 `Proceed` 并返回结果 | 继续并返回下一跳结果 |
| 修改下一跳结果 | 转换 Response |
| 不调用 `Proceed` 并返回结果 | 当前插件结束流水线 |
| 不调用 `Proceed` 并返回错误 | 终止 Call |
| 重复调用 `Proceed` | 返回 `flow_already_proceeded` |

现有 Provider 可以直接返回结果，不需要调用 Proceed。

### 5.5 Request、Response 和 Event

Request 按 Priority 递减传播。Response 沿 Request 路径反向返回：

```text
Request:  caller → session → sandbox → tools
Response: tools → sandbox → session → caller
```

新建 Event 按 Priority 递减传播。属于在途 Call 的流式 Event 沿 Request 返回路径传播。

Response 中的 `priority` 只记录流水线位置，不重新选择 Consumer。

### 5.6 Host 内部

Host 可以记录当前 Priority、已访问插件、返回路径和跳数。这些信息属于内部实现，不进入插件接口。

流水线必须检测循环并限制最大跳数。

### 5.7 验收

1. 同一 Method 的 Consumer 按 Priority 顺序执行。
2. Priority 相同时显式冲突。
3. 每个 Consumer 最多调用一次 Proceed。
4. Response 按 Request 路径反向返回。
5. 流式 Event 不会绕过 Request 消费者。
6. Plugin 不能伪造下一跳 Priority。
7. 循环和最大跳数均有测试。

Frame Priority 切片完成本节验收。

## 6. 插件体系

### 6.1 初始插件

| 插件 | 角色 |
|---|---|
| `agent` | Turn、Step、LLM 和 Tool 编排 |
| `llm-openai` | OpenAI-compatible Completion |
| `session` | Session Fact Log 和 UI |
| `model-context` | Model Context 构建和压缩 |
| `tools` | Tool 注册、发现和分发 |
| `sandbox` | 高危 Frame 检查 |
| `file-tools` | 文件操作 |
| `shell-tools` | Shell 操作 |
| `web-tools` | 搜索和网页读取 |

不预先创建 `skills`、`mcp`、`memory`、`telemetry` 或 `browser-tools` 插件。

### 6.2 内部模块

`model-context` 内部先处理：

- System Prompt；
- Skill 读取；
- Memory 检索；
- MCP 适配；
- Token Budget；
- Context 压缩。

这些内容暂时是模块，不形成独立插件或平台公共接口。

以后满足以下条件时再拆分：

| 内容 | 拆分条件 |
|---|---|
| Memory | 需要独立存储、索引或生命周期 |
| Skills | 需要独立安装、市场或权限 |
| MCP | 需要独立连接管理、UI 或多个实现 |
| Telemetry | Session 无法满足 Trace 和指标需求 |

### 6.3 依赖

```text
agent
├─ depends_on model-context
├─ depends_on llm-openai
└─ depends_on tools

model-context
├─ depends_on session
└─ requires tools

file-tools、shell-tools、web-tools
└─ requires tools
```

`tools` 不硬依赖具体 Tool Provider。各 Tool Provider 通过弱依赖注册 Tool。

## 7. Model Context

### 7.1 真源

资源定义由 Skill 文件、MCP 配置、Memory 索引和插件配置保存。模型实际输入写入 Session。

每次构建后追加 `context_snapshot` Fact。压缩结果追加 `context_compaction` Fact。

Session 因此能够回答某次 Turn 实际向模型发送了什么。

### 7.2 Snapshot

只使用一个 Model Context Snapshot：

| 字段 | 含义 |
|---|---|
| `model_context_id` | 快照标识 |
| `session_id` | 所属 Session |
| `turn_id` | 所属 Turn |
| `revision` | 构建版本 |
| `messages` | 实际模型消息 |
| `tools` | 实际 Tool Definition |
| `parts` | 内容来源和顺序 |
| `token_estimate` | Token 估算 |
| `hash` | 快照摘要 |
| `warnings` | 截断和降级说明 |

`parts` 是 Snapshot 的内部结构。平台不引入 Context Block、Context Contribution 或 Context Recipe。

`model_context_id` 不等于 Host 的 `context_id`。

### 7.3 构建

```text
读取 Session
→ 读取 Skill、MCP 和 Memory
→ 获取 Tool Definition
→ 组装 System Prompt
→ 去重
→ 应用 Token Budget
→ 可选压缩
→ 写入 context_snapshot
→ 返回给 Agent
```

压缩不改写 Session Fact Log。`context_compaction` 记录来源 Seq、摘要、策略和 Token 变化。

主要能力只有：

- `model-context.build`
- `model-context.inspect`
- `model-context.compact`

高效上下文是 `build` 的内部策略。

## 8. Tools

`tools` 是 Tool 领域的注册与分发插件。它不是通用 Service Broker。

只提供四个能力：

| 能力 | 用途 |
|---|---|
| `tools.register` | 注册 Tool |
| `tools.unregister` | 注销 Tool |
| `tools.list` | 返回 Tool Definition |
| `tools.call` | 根据 Tool 名称执行 |

Tool Definition 包含：

| 字段 | 含义 |
|---|---|
| `name` | 模型看到的 Tool 名称 |
| `description` | 模型看到的说明 |
| `input_schema` | 参数 Schema |
| `source_plugin` | 提供方 |
| `target_capability` | Host 分发 Capability |
| `target_method` | Host 分发 Method |
| `risk` | `low`、`medium` 或 `high` |

`target_capability` 和 `target_method` 不发送给模型。

`tools` 不硬依赖具体 Tool Provider。Tool Provider 注册后才出现在 `tools.list`。

## 9. Tool Provider

### 9.1 file-tools

计划能力：

- `file.read`
- `file.write`
- `file.list`
- `file.stat`
- `file.search`

路径使用 Workspace 相对路径。

### 9.2 shell-tools

计划能力：

- `shell.execute`
- `shell.cancel`
- `shell.status`

执行使用 Go `context.Context`。取消时终止子进程。

### 9.3 web-tools

计划能力：

- `web.search`
- `web.fetch`
- `web.cancel`

搜索提供方以后可以细分，当前保持统一 `web` Capability。

## 10. Sandbox

Sandbox 是 Consumer，不是 Tool Provider。它至少消费：

| `(capability, method)` | 检查内容 |
|---|---|
| `tools.call` | 模型 Tool Call |
| `file.write` | 文件修改 |
| `shell.execute` | 命令执行 |
| `web.fetch` | 网络读取 |
| `web.search` | 搜索内容 |

只检查 `tools.call` 不够。其他插件可以直接调用 `shell.execute` 并绕过 Tool 层。

Sandbox 可以允许、修改、拒绝或转为待确认状态。它不负责实际执行文件、Shell 和网络操作。

## 11. Priority

`priority` 是插件自身参数，不做范围限制。建议值只作为开发参考：

| 插件 | Priority |
|---|---:|
| `session` | `1000` |
| `sandbox` | `500` |
| `tools` | `0` |
| Tool Provider | `0` |

同一 `(capability, method)` 的 Consumer Priority 必须唯一。不同 Method 可以使用相同 Priority。

不提供 Priority 分段、优先级角色或动态调整规则。

## 12. 暂不实现

1. Effect Ledger 和 Resource Ledger。
2. 配置热替换、滚动替换和配置 reconciliation。
3. Capability Grant、权限系统和外部安全沙箱。
4. Service Broker、多 Provider 和负载均衡。
5. `secret-store`。
6. 独立的 `skills`、`mcp`、`memory`、`telemetry` 插件。
7. Method 级依赖声明。
8. 插件市场和跨主机 Provider。
9. 在线迁移运行中 Call。
10. 业务副作用的通用数学逆操作。
11. 自动推导未声明的 Capability 依赖。

现有进程隔离继续作为故障边界，不扩展为安全沙箱。

## 13. 实施顺序

| 顺序 | 切片 | 验收重点 |
|---|---|---|
| 1 | Provider 生命周期 | 注册完成、Generation 和五态模型 |
| 2 | Binding Generation | 重启后 Binding 不误用 |
| 3 | Context Call | Call 与 Handler Context |
| 4 | Cancel Frame | 取消传播和唯一终态 |
| 5 | Frame Priority | Priority、`consumes` 和 Proceed |
| 6 | Tools | 四个 Tool 能力 |
| 7 | Model Context | Snapshot 与 Session Fact |
| 8 | Tool Provider | file、shell、web |
| 9 | Sandbox | 高危 Frame 消费 |

下一轮从第 6 项 Tools 开始。

实现时一次只完成一个可验收切片。每个切片完成后运行 `go test ./...`，并同步更新对应平台文档。
