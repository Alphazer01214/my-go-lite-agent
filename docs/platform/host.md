# internal/host

`internal/host` 是 Host 内核的实现。它是平台契约的落地，不新增任何契约。

线契约见 [../protocol.md](../protocol.md)。本文只讲实现：数据结构、控制流、不变量和边界。

## 1. 职责边界

Host 负责：

- 发现结果的挂载与进程生命周期；
- 路由表与 Consumer 流水线；
- pending 闭环与 id 改写；
- 取消转发与宽限期；
- 执行上下文与 Plugin Instance；
- 插件与实例快照；
- 管理面（重新扫描、启停）；
- Host 主动发起的 Call；
- UI 发布的转发。

Host 不负责：

- 解析 Manifest 语义。扫描在 `internal/plugin` 完成；
- 解释插件业务 payload；
- 决定 State 与 Command 的含义；
- 读写插件配置文件；
- 实现浏览器协议。传输在 `internal/webui` 完成。

## 2. 文件与职责

| 文件 | 职责 |
|---|---|
| `host.go` | `Host` 结构、实例索引、pending 表、关闭流程 |
| `transport.go` | 帧路由、注册、转发、事件投递、Host 方法分派 |
| `pipeline.go` | Consumer 流水线、候选选择、`host.proceed` |
| `runtime.go` | Plugin Instance、执行上下文、错误码映射 |
| `call.go` | Host 主动发起的 Call |
| `cancel.go` | 取消转发与宽限期 |
| `proc.go` | 进程启动、entry 解析、死亡闭环 |
| `lifecycle.go` | 重新扫描、启停接口 |
| `snapshot.go` | 插件与实例快照 |
| `ui.go` | UI 发布接口 |
| `constant.go` | 帧常量、Host 方法名、错误哨兵、时间常量 |

## 3. 核心数据结构

### 3.1 Host

```go
type Host struct {
    mu sync.RWMutex
    plugins map[string]*proc       // instance id -> 进程
    pending map[string]*wait       // fwd-N / host-N -> 在途调用
    routes  map[string]string      // "capability.method" -> 属主插件名
    cancelGrace time.Duration
    disabled map[string]bool       // 插件名 -> 是否禁用
    lifecycleMu sync.Mutex
    discoveries plugin.Discoveries
    pluginsDir  string
    seq    int                     // 共享自增计数器
    closed bool
    gen    map[string]int          // instance id -> 进程代际
    contexts   map[string]*ExecutionContext
    contextSeq int
    mountedUI map[string]bool
    hlog        func(string)
    uiPublisher UIPublisher
    trace       TraceFunc
}
```

| 字段 | 说明 |
|---|---|
| `plugins` | 键是 Plugin Instance id，值是进程句柄 |
| `pending` | 键是 Host 分配的 id，值是在途调用的状态 |
| `routes` | 键是 `capability.method`，值是属主插件名 |
| `disabled` | 只有测试会写入。生产路径没有禁用入口 |
| `gen` | 每个实例的进程代际计数，只增不减 |
| `contexts` | 执行上下文表，懒初始化 |
| `mountedUI` | 已声明但当前没有任何代码读写 |
| `seq` | `fwd-N`、`host-N` 和自动实例 id 共享的计数器 |

`disabled` 没有对外的写入口。`PluginSnapshot.Enabled` 因此在生产路径恒为真。`capability_conflict` 和 `plugin_disabled` 两个错误码也因此不可达。

### 3.2 proc

```go
type proc struct {
    discovery      plugin.Discovery
    instanceID     string
    pluginName     string
    configRevision string
    cmd            *exec.Cmd
    stdin          io.WriteCloser
    gen            int
    mu             sync.Mutex
    done           chan struct{}
    state          string
    registrations  []RegistrationSnapshot
    failureCode    string
    failureMessage string
    timeout        time.Duration
}
```

`instanceID` 是进程激活身份，`pluginName` 是 Frame 身份。两者不同，`from` 注入用后者。

`stdin` 是唯一能往插件写 Frame 的通道。写它必须持 `proc.mu`。

`done` 在 `cmd.Wait()` 返回后关闭。`Close` 用它等待进程真正退出。

`timeout` 字段当前没有写入点。

### 3.3 wait

```go
type wait struct {
    caller           string
    callerPlugin     string
    target           string
    targetPlugin     string
    targetGeneration int
    contextID        string
    frameID          string
    kind             WaitKind
    events           []*Frame
    capability       string
    method           string
    payload          json.RawMessage
    cancelReason     string
    cancelTimer      *time.Timer
    pipeline         *pipelineState
    targetPriority   int
    resCh   chan *Frame
    onEvent func(*Frame)
}
```

| 字段 | 说明 |
|---|---|
| `caller` | 调用方的实例 id |
| `callerPlugin` | 调用方的插件名。取消帧的 `from` 用它 |
| `target` | 被调方的实例 id |
| `targetPlugin` | 被调方的插件名。应答的 `from` 用它 |
| `targetGeneration` | 转发时的目标代际。取消前用于校验 |
| `frameID` | 调用方的原始帧 id。回程时恢复它 |
| `kind` | `WaitPlugin` 或 `WaitHost` |
| `events` | `WaitHost` 且没有 `onEvent` 时缓存的归因 Event |
| `payload` | 转发时的原始 payload 副本 |
| `pipeline` | 非空表示这跳在流水线里 |
| `resCh` | `WaitHost` 的结果通道，容量 1 |
| `onEvent` | `WaitHost` 的事件回调 |

### 3.4 WaitKind

| 值 | 含义 | 结果去向 |
|---|---|---|
| `WaitPlugin` | 插件 A 调插件 B | 写回插件 A 的 stdin |
| `WaitHost` | Host 主动发起或 `host.proceed` 产生 | 投 `resCh` 或 `onEvent` |

`WaitHost` 的应答永远不会写回任何插件。这是 `WaitHost` 与 `WaitPlugin` 最主要的分叉。

### 3.5 pipelineState

```go
type pipelineState struct {
    mu           sync.Mutex
    capability   string
    method       string
    contextID    string
    nextPriority int
    hops         int
    visited      map[string]bool
    proceeded    map[string]bool
}
```

`nextPriority` 是下一跳的优先级上界。初值是 `FramePriorityMax`。每次选中候选后降为该候选优先级减一。

`visited` 记录已访问的实例 id，用于检测回环。`proceeded` 记录已 Proceed 过的 Call id。

`pipelineState` 由同一次业务调用的所有跳共享。它挂在 pending 上，跨 goroutine 使用，因此自带锁。

## 4. 常量

| 常量 | 值 | 用途 |
|---|---|---|
| `FramePriorityMax` | 2147483647 | 流水线初始优先级上界 |
| `PipelineMaxHops` | 64 | 流水线最大跳数 |
| `DefaultShutdownGrace` | 2 秒 | 停机等待在途 Call 的宽限期 |
| `DefaultCancelGrace` | 2 秒 | 取消等待终态的宽限期 |
| `ForwardIDPrefix` | `fwd-` | 转发 id 前缀 |
| `HostCallIDPrefix` | `host-` | Host 主动调用 id 前缀 |
| `FromHost` | `host` | Host 合成帧的 `from` |
| `HostCapability` | `host` | Host 方法的 capability |

Provider 状态只有五个：`inactive`、`loading`、`active`、`stopping`、`failed`。

## 5. 生命周期

### 5.1 构造

`New(discoveries, logFunc)` 只构造结构体，不启动任何进程。

`logFunc` 为 `nil` 时日志走 stderr。stdout 留给对话界面。

### 5.2 启动

`Start()` 读取 autostart 闭包并逐个挂载。

```text
plugin.AutostartClosure(discoveries)
  → ordered, warns
  → warns 逐条记日志
  → 逐个 launchInstance(name, discovery, "")
```

挂载是软失败。单个插件启动失败只记日志，Host 继续挂载下一个。

### 5.3 进程启动

`launchInstance(instanceID, discovery, configRevision)`：

1. `instanceID` 为空时返回 `ErrBadArguments`。
2. 插件被禁用时返回 `ErrPluginDisabled`。
3. `resolveEntry` 解析出绝对可执行路径。
4. `cmd.Dir` 设为插件目录的绝对路径。
5. `cmd.Env` 继承当前环境，`cmd.Stderr` 接 stderr。
6. 实例 id 已存在时返回 `ErrInstanceExists`，并关掉刚建的管道。此时还没有分配 `gen`。
7. 分配 `gen`，写入 `plugins` 表，状态设为 `loading`。
8. `cmd.Start()` 失败时状态转 `failed`，失败码 `plugin_down`。
9. 启动后开一个 goroutine 跑 `cmd.Wait()`，回收进程并关闭 `done`。
10. 再开一个 goroutine 跑 `read()`，持续读该进程的 stdout。

`resolveEntry` 的规则：entry 相对插件目录解析；无扩展名时再试一次 `.exe`；最后转成绝对路径。

绝对路径是必需的。`cmd.Dir` 非空时，相对 `Path` 会按子进程工作目录再解析一次而找不到。

`cmd.Dir` 必须是插件目录。插件按 CWD 读 `config.json`，把会话写进 `./sessions`。

### 5.4 关闭

`Close()` 的顺序：

1. 置 `closed` 标志，收集 pending 键和进程列表。
2. 对每个 pending 调 `cancelPending`，把取消帧发出去。
3. 清空 pending 表，对 `WaitHost` 合成 `server_closed` 应答，对 `WaitPlugin` 写回错误应答。
4. 关闭所有插件的 stdin。EOF 让插件 SDK 的接收循环退出。
5. 等待 `DefaultShutdownGrace`。超时的进程被强制杀死。

`Close` 可重复调用。第二次直接返回。

## 6. 路由

### 6.1 入口

`routeFrame(instanceID, gen, frame)` 先做代际校验，再调 `route`。

代际不符的帧被丢弃并记日志。这条规则挡住旧进程激活的迟到帧。

`route` 按 `type` 分派：

| 帧类型 | 处理 |
|---|---|
| `req` | `handleRequest` |
| `res` | `handleResponse` |
| `evt` | `handleEvent` |
| `cancel` | `handleCancel` |
| 其它 | 忽略 |

### 6.2 handleRequest

顺序如下：

1. 帧带 `context_id` 时，先校验调用方是上下文参与者。失败回 `context_participant_mismatch`。
2. Host 已关闭时回 `host_closed`。
3. `capability` 是 `host` 时转 `dispatchHost`。
4. `capability` 或 `method` 为空时回 `method_not_found`。
5. 把 `priority` 置为 `FramePriorityMax`，建 `pipelineState`。
6. 选流水线候选。
7. 选不到候选时回退到路由表属主。属主也没有时回 `method_not_found`。
8. 转发。

### 6.3 路由表

路由表的新增入口只有一个，就是 `host.register`。

删除入口也只有一个，就是 `unregisterPlugin`。它删掉某个插件名拥有的全部路由键。`StartPlugin` 和 `StopPlugin` 各调用一次，让新进程能重新注册同一批键。

判定规则：

| 情况 | 结果 |
|---|---|
| `capability` 或 `method` 为空 | 进 `rejected` |
| 键已有别的属主，本插件不声明 `consumes` | 进 `rejected`，属主不变 |
| 键已有别的属主，本插件声明了 `consumes` | 进 `accepted`，作为 Consumer 参与 |
| 键没有属主，本插件既不声明 `provides` 也不声明 `consumes` | 本插件成为属主 |
| 键没有属主，本插件不声明 `provides` 但声明了 `consumes` | 进 `accepted`，路由归属保持为空 |
| 键没有属主，本插件声明了 `provides` | 本插件成为属主 |

同插件重复注册同键不冲突，采用最后一次。

`provides` 不是路由的兜底来源。只有 `host.register` 上报才会写路由表。

注册成功后实例状态转 `active`，失败信息清空，注册项按 `capability`、`method` 排序保存。

注册的失败情形：

| 情形 | 错误码 | 附带效果 |
|---|---|---|
| payload 不是合法 JSON | `bad_payload` | 实例转 `failed` |
| 上报方不是已知实例 | `plugin_not_mounted` | 无 |
| 实例不在 `loading` 或 `active` | `plugin_not_ready` | 无 |

### 6.4 流水线候选选择

`selectPipelineCandidate(state)` 的过滤条件：

1. 跳数达到 `PipelineMaxHops` 时返回 `pipeline_hops_exceeded`。
2. 候选实例必须是 `active` 状态。
3. 候选要么注册过该键，要么在 Manifest 的 `consumes` 里声明了该方法。
4. 首跳且存在 Consumer 声明时，只考虑 Consumer。
5. 已访问的实例被跳过。
6. 优先级高于 `nextPriority` 的候选被跳过。

选中的是优先级最高者。出现多个同优先级候选时返回 `consume_priority_conflict`。

选不到候选时：

- 存在已访问的实例，返回 `pipeline_loop`；
- 否则返回 `method_not_found`。

选中后：标记已访问，跳数加一，`nextPriority` 降为该候选优先级减一。

流水线因此严格按优先级降序推进。同优先级的两个 Consumer 无法共存。

### 6.5 转发

`forwardPipeline` 的顺序：

1. `acceptingNewCall` 校验目标实例是 `active`。否则 `plugin_not_ready`。
2. `joinExecutionContext` 把目标实例加入上下文参与者集合。
3. 分配 `fwd-N` id，建立 `WaitPlugin` 的 pending。
4. 改写帧：`id` 换成 `fwd-N`，`from` 换成调用方插件名，`priority` 换成候选的 Manifest 优先级。
5. 写目标实例的 stdin。写失败时删除 pending 并返回错误。

转发帧的 `priority` 是候选的 Manifest 优先级，不是 `FramePriorityMax`。

### 6.6 缺席即放行

流水线选不到候选时，`handleRequest` 回退到路由表属主并直接转发。

这条回退的优先级取属主的 Manifest 优先级，跳数记为 1。

效果是：声明了 `consumes` 但当前没有活跃 Consumer 时，Host 跳过这一跳。只有 Consumer 和 Provider 都不存在时，调用才返回 `method_not_found`。

## 7. 应答与事件

### 7.1 handleResponse

1. 按帧 id 查 pending。查不到时记日志并返回错误。
2. 删除 pending，停止取消定时器。
3. 复制帧，恢复调用方 id，`from` 填被调方插件名，`context_id` 填 pending 记录的上下文，`priority` 回填目标跳的优先级。
4. `WaitHost` 投 `resCh`。`WaitPlugin` 写回调用方 stdin。

`WaitHost` 的应答不写回任何插件。

`resCh` 容量是 1，且每个关闭路径只写一次。通道满说明应答已经到达，这次是多余的。

### 7.2 handleEvent

1. 帧 id 为空时按广播处理，记日志后丢弃。
2. 按帧 id 查 pending。查不到时记日志后丢弃。
3. 复制帧，恢复调用方 id，`from` 填事件来源插件名。
4. `WaitHost` 走 `onEvent` 回调，没有回调时缓存进 `events`。
5. `WaitPlugin` 写回调用方 stdin。

### 7.3 广播

无 id 的 Event 记一条 `drop broadcast evt` 日志后丢弃。Host 没有通用广播扇出。

## 8. 取消

### 8.1 cancelPending

1. 先按帧 id 查 pending。查不到时按 `frameID` 匹配查。
2. 查不到时返回 `ErrTimeout`。
3. 目标代际与 pending 记录不一致时返回 `ErrContextBinding`。
4. 第一次取消时记录原因并启动 `cancelGrace` 定时器。
5. 写 `cancel` 帧到目标实例。帧的 `id` 是 pending 键，`from` 是调用方插件名。

写帧失败时删除 pending 并让等待失败。

取消帧发出后 pending 保留。Host 继续等目标方的终态应答，或等宽限期到期。

### 8.2 宽限期

定时器到期时调 `cancelTimeout`：

- pending 已消失则什么都不做。
- 代际或原因变了则什么都不做。
- 否则删除 pending，合成终态应答。

终态应答的错误码是 `call_cancelled` 或 `call_deadline_exceeded`。

### 8.3 handleCancel

插件发来的 `cancel` 帧按 `frameID` 和调用方实例 id 匹配 pending。匹配到后转 `cancelPending`。

原因不是 `deadline_exceeded` 时一律归一为 `cancelled`。

## 9. 执行上下文

### 9.1 结构

```go
type ExecutionContext struct {
    ID           string
    Owner        string
    RootID       string
    ParentID     string
    Bindings     map[string]ContextBinding
    Participants map[string]bool  // json:"-"，不出现在应答里
    Closed       bool
}

type ContextBinding struct {
    Capability string
    InstanceID string
    Generation int
}
```

`Participants` 带 `json:"-"`。它不进入任何应答。

### 9.2 创建

`CreateExecutionContext(owner, parentID, bindings)`：

1. `owner` 为空时返回 `ErrBadArguments`。
2. Host 已关闭时返回 `ErrHostClosed`。
3. 逐条校验 `bindings`：

| 情形 | 错误 |
|---|---|
| Capability 或实例 id 为空 | `context_binding_invalid` |
| 实例不存在 | `instance_not_found` |
| 该 Capability 已有路由属主，且目标实例的插件名不是属主 | `context_binding_invalid` |
4. 有 `parentID` 时：父上下文必须存在，否则 `context_not_found`；父上下文已关闭时 `context_closed`；创建者既不是父属主也不是父参与者时 `context_participant_mismatch`。
5. 子上下文继承父的绑定和参与者集合。同名绑定被子上下文自己的覆盖。
6. 分配 `ctx-N` 形式的 id。根上下文的 `RootID` 等于自身 id。

绑定建立时记录目标实例当前的 `gen`。这是配置代隔离的落点。

### 9.3 改绑定

`BindExecutionContext(owner, contextID, capability, instanceID)`：

| 情形 | 错误 |
|---|---|
| 任一参数为空 | `ErrBadArguments` |
| 上下文不存在 | `ErrContextNotFound` |
| 上下文已关闭 | `ErrContextClosed` |
| 调用方不是属主 | `ErrContextOwner` |
| 实例不存在 | `ErrInstanceNotFound` |
| 该 Capability 已有属主且目标实例不是属主 | `ErrContextBinding` |

改绑定会按目标实例当前的 `gen` 重新固定 Generation。

### 9.4 关闭

`CloseExecutionContext(owner, contextID)`：

1. 上下文不存在时返回 `ErrContextNotFound`。
2. 调用方不是属主时返回 `ErrContextOwner`。
3. 递归关闭全部子上下文，从表中删除。
4. 对每个被关闭的上下文，先取消 pending，再把 pending 失败为 `context_closed`。

关闭后查询子上下文返回 `context_not_found`。

### 9.5 使用上下文

`useExecutionContext(contextID, instanceID)` 校验上下文存在、未关闭，调用方是参与者。校验通过后把调用方加入参与者集合。

`joinExecutionContext` 只做加入，不校验参与者身份。它用在转发路径上。

### 9.6 解析目标实例

`resolveInstance(contextID, capability, method, defaultPlugin)`：

1. 上下文存在时查 `bindings[capability]`。
2. 绑定实例不存在时返回 `ErrInstanceNotFound`。
3. 绑定实例的 `gen` 与记录的 Generation 不一致时返回 `ErrContextBinding`。
4. 该键的路由属主与绑定实例的插件名不一致时返回 `ErrContextBinding`。
5. 命中绑定时返回绑定实例。
6. 否则返回默认属主。

Provider 重启后旧 Generation 的绑定立即失效。重新 `host.context.bind` 固定到新 Generation。

## 10. Plugin Instance

### 10.1 创建

`CreatePluginInstance(pluginName, instanceID, configRevision)`：

| 情形 | 错误 |
|---|---|
| `pluginName` 为空 | `ErrBadArguments` |
| Host 已关闭 | `ErrHostClosed` |
| 实例 id 已存在 | `ErrInstanceExists` |
| 插件未被发现 | `ErrPluginNotMounted` |
| 插件被禁用 | `ErrPluginDisabled` |

`instanceID` 为空时自动生成 `<plugin_name>:instance:<N>`，`N` 取自 Host 的共享计数器。

默认实例以插件名作为实例 id。

### 10.2 关闭

`ClosePluginInstance(instanceID)`：

1. 实例不存在时返回 `ErrInstanceNotFound`。
2. 已在 `stopping` 时直接返回成功。
3. 置 `stopping`，等待在途 Call 完成，宽限期 `DefaultShutdownGrace`。
4. 超时的 pending 失败为 `plugin_down`。
5. 停进程，标不健康，从表中删除实例。

### 10.3 重启

`RestartPluginInstance(instanceID)` 的前四步与关闭相同，之后用同一实例 id 重新启动。

实例不在表中时返回 `ErrInstanceNotFound`，正在停止时返回 `ErrInstanceClosed`。

重启保留 `configRevision`，`gen` 加一。新进程在注册前状态是 `loading`。

## 11. 快照

### 11.1 PluginSnapshot

```go
type PluginSnapshot struct {
    Name               string
    Version            string
    State              string
    SelectedInstanceID string
    Instances          []PluginInstanceSnapshot
    Enabled            bool
    Missing            bool
    Provides           []string
    DependsOn          []string
}
```

快照按插件名排序。实例按实例 id 排序。

`selected_instance_id` 的选取规则是先按状态权重，再按实例 id 升序：

| 状态 | 权重 |
|---|---|
| `active` | 0 |
| `failed` | 1 |
| `loading` | 2 |
| `stopping` | 3 |
| `inactive` | 4 |

没有任何实例时，状态是 `inactive`，`selected_instance_id` 是空串。

只出现在 `depends_on` 里、自身未被发现的插件名，会被合成为一个条目：状态 `inactive`，`missing` 为真，`version` 为空。

`autostart: true` 不影响状态。没有实例的插件仍然是 `inactive`。

快照不包含插件目录或可执行文件路径。

### 11.2 PluginInstanceSnapshot

```go
type PluginInstanceSnapshot struct {
    InstanceID     string
    PluginName     string
    ConfigRevision string
    State          string
    Generation     int
    Registrations  []RegistrationSnapshot
    FailureCode    string
    FailureMessage string
}
```

`InstanceSnapshot()` 按实例 id 排序。

### 11.3 RegistrationSnapshot

```go
type RegistrationSnapshot struct {
    Capability  string
    Method      string
    Name        string
    Description string
}
```

`Name` 和 `Description` 来自插件的 `WithInfo`，只用于展示。

## 12. 管理面

`PluginLifecycle` 接口是 Host 暴露给 WebUI 的窄缝：

```go
type PluginLifecycle interface {
    PluginSnapshot() []PluginSnapshot
    RescanPlugins() (plugin.Discoveries, error)
    StartPlugin(name string) error
    StopPlugin(name string) error
}
```

### 12.1 重新扫描

`RescanPlugins` 是运行期唯一一次目录搜索。

它更新发现结果，但**不自动启动新发现的插件**。

插件目录未配置时返回 `ErrBadArguments`，Host 已关闭时返回 `ErrHostClosed`。

### 12.2 启动

`StartPlugin(name)`：

1. 名字为空时返回 `ErrBadArguments`。
2. Host 已关闭时返回 `ErrHostClosed`。
3. 插件未被发现时返回 `ErrPluginNotMounted`。
4. 插件已 `active` 时直接返回成功。
5. 逐个检查 `depends_on`。不健康时只记一条 `start warn` 日志，不阻止启动。
6. 已有旧进程时先拆掉。
7. 提前注销该插件的路由，让新进程能重新注册同一批键。
8. 启动进程。

`depends_on` 只用来提示，不用来许可。缺依赖照样启动，后续调用自己拿到 `plugin_not_mounted`。

### 12.3 停止

`StopPlugin(name)`：

1. 名字为空时返回 `ErrBadArguments`。
2. Host 已关闭时返回 `ErrHostClosed`。
3. 插件未被发现时返回 `ErrPluginNotMounted`。
4. 把该插件名下所有实例置 `stopping`。
5. 等待这些实例上的在途 Call 完成，宽限期 `DefaultShutdownGrace`。
6. 超时的 pending 失败为 `plugin_down`。
7. 停进程，标不健康，从表中删除实例。
8. 注销该插件的路由。

停止完成后该插件的路由被注销，后续调用返回 `method_not_found`。停止窗口内的调用返回 `plugin_not_ready`。

停止不级联到依赖方。

## 13. Host 主动调用

`CallWithEvents` 与 `CallWithEventsInContext` 是 Host 发起业务调用的入口。

### 13.1 前置校验

| 情形 | 错误 |
|---|---|
| `ctx` 为 nil | `ErrBadArguments` |
| `ctx` 已结束 | 包装后的 ctx 错误 |
| Host 已关闭 | `ErrHostClosed` |
| `from`、`capability` 或 `method` 为空 | `ErrBadArguments` |
| `capability` 是 `host` | `ErrMethodNotFound` |
| 路由表没有该键 | `ErrMethodNotFound` |

`capability` 是 `host` 时直接拒绝。即使路由表里有 `host.plugins` 也不放行。

被拒绝的调用不建立 pending，也不向插件写任何帧。

### 13.2 执行

1. 解析目标实例。上下文绑定优先。
2. `acceptingNewCall` 校验目标实例是 `active`。
3. 分配 `host-N` id，建立 `WaitHost` 的 pending，`resCh` 容量 1。
4. 写请求帧。`from` 原样填调用方给的字符串。`context_id` 填上下文 id。
5. 等 `resCh` 或 `ctx.Done()`。

`ctx` 结束时先尝试 `cancelPending`。取消成功后仍等 `resCh`，让目标方的终态应答决定错误码。取消失败时先做一次非阻塞读取，应答已到达就返回该应答，否则返回取消错误。

### 13.3 事件

`onEvent` 回调在插件读循环线程上执行，必须快速且非阻塞。

`onEvent` 为 nil 是合法的。事件被丢弃，调用照常完成。

事件按到达顺序投递，不重排。终态应答在事件之后。

## 14. UI 发布

`UIPublisher` 是 Host 与 WebUI 之间的窄缝：

```go
type UIPublisher interface {
    PublishState(from string, payload json.RawMessage) (json.RawMessage, error)
    PublishCommand(from string, payload json.RawMessage) (json.RawMessage, error)
}
```

`host.ui.state` 和 `host.ui.command` 都走这里。

Host 传给 publisher 的 `from` 是发起方的 **Plugin Instance id**，不取自 payload。

这与帧上的 `from` 不同。帧上的 `from` 是插件名，publisher 上的 `from` 是实例 id。

后果是只有以插件名作为实例 id 的默认实例能发布 UI。带实例后缀的次要实例发布时会因为找不到 Mount 而被拒。

publisher 返回的 payload 原样作为应答 payload 回给插件。

没有 publisher 时返回 `ui_not_mounted`。

错误码映射：

| publisher 错误 | 错误码 |
|---|---|
| `protocol.ErrUINotMounted` | `ui_not_mounted` |
| `protocol.ErrUIOwnerMismatch` | `ui_owner_mismatch` |
| `ErrBadPayload` | `bad_payload` |
| `ErrBadArguments` | `bad_arguments` |
| 其它 | `handler_error` |

## 15. 错误码映射

`ErrorCode(err, fallback)` 把哨兵错误映射为稳定错误码。顺序即优先级。

| 哨兵 | 错误码 |
|---|---|
| `ErrMethodNotFound` | `method_not_found` |
| `ErrHostClosed` | `host_closed` |
| `ErrServerClosed` | `server_closed` |
| `ErrBadArguments` | `bad_arguments` |
| `ErrBadPayload` | `bad_payload` |
| `ErrTimeout` | `timeout` |
| `ErrRouteFailed` | `route_failed` |
| `ErrPluginNotMounted` | `plugin_not_mounted` |
| `ErrPluginDown` | `plugin_down` |
| `ErrPluginDisabled` | `plugin_disabled` |
| `ErrPluginNotReady` | `plugin_not_ready` |
| `ErrConsumePriority` | `consume_priority_conflict` |
| `ErrFlowProceeded` | `flow_already_proceeded` |
| `ErrPipelineLoop` | `pipeline_loop` |
| `ErrPipelineHops` | `pipeline_hops_exceeded` |
| `context.DeadlineExceeded` | `call_deadline_exceeded` |
| `context.Canceled` | `call_cancelled` |
| `ErrContextNotFound` | `context_not_found` |
| `ErrContextClosed` | `context_closed` |
| `ErrContextOwner` | `context_owner_mismatch` |
| `ErrContextParticipant` | `context_participant_mismatch` |
| `ErrContextBinding` | `context_binding_invalid` |
| `ErrInstanceNotFound` | `instance_not_found` |
| `ErrInstanceExists` | `instance_exists` |
| `ErrInstanceClosed` | `instance_closed` |
| 其它 | `fallback` |

## 16. 死亡闭环

`markUnhealthy(instanceID, gen)` 在 stdout 读失败时调用。

1. Host 已关闭时直接返回。
2. 实例不存在、代际不符或状态已是 `inactive` 时返回。
3. 状态是 `stopping` 时转 `inactive` 并返回。
4. 否则转 `failed`，失败码 `plugin_down`，写失败信息，记日志。

随后调 `failInstancePending`：

- 该实例作为目标的 pending 全部失败为 `plugin_down`。
- 该实例作为调用方的 pending 从表中移除。`WaitHost` 的等待方收到 `plugin_down`。`WaitPlugin` 的调用方已经是死进程，不再回写。

`failProviderRegistration` 在注册 payload 解析失败时把实例转 `failed`，清空注册项。

## 17. 锁与并发

| 锁 | 保护范围 |
|---|---|
| `Host.mu` | 实例索引、路由表、pending 表、上下文表、快照数据 |
| `Host.lifecycleMu` | 串行化重新扫描、启动、停止 |
| `proc.mu` | 单个实例的 stdin 写入 |
| `pipelineState.mu` | 流水线游标、已访问集合、Proceed 记录 |

约定：

- 读路径用 `RLock`，写路径用 `Lock`。
- 任何路径都不在持有 `proc.mu` 时去拿 `Host.mu`。
- 写回插件的合成应答一律在锁外发出。
- `trace` 回调在锁外调用。
- `host.proceed` 的后续处理在独立 goroutine 里跑，不阻塞读循环。

## 18. Trace

`TraceFunc` 的签名是 `func(dir, plugin string, frame *Frame)`。

`dir` 为 `in` 或 `out`。回调在锁外调用，必须快且非阻塞。

`cmd/liteagent-server` 用它打印每条过界帧。payload 超过 200 字节时按字符边界截断。

## 19. 测试覆盖

| 领域 | 覆盖情况 |
|---|---|
| 路由表与注册 | 已覆盖。含冲突、last-wins、Consumer 不抢属主、`provides` 不作兜底 |
| 流水线 | 已覆盖优先级降序、同优先级冲突、回环、跳数上限 |
| Proceed | 已覆盖转发下一跳、不阻塞读循环、事件与应答的 id 还原 |
| id 改写与 `from` | 已覆盖 `fwd-N`、调用方 id 还原、`from` 注入 |
| 代际 | 已覆盖旧代际帧被丢弃、旧代际取消被拒 |
| 执行上下文 | 已覆盖继承、绑定属主校验、Generation 固定、重绑定、关闭子树、参与者校验 |
| Plugin Instance | 已覆盖快照字段、重启加代际、未知插件 |
| 取消 | 已覆盖转发、保留 pending、宽限期合成终态、期限与取消的区分 |
| 死亡闭环 | 已覆盖 `plugin_down` 合成与状态转 `failed` |
| 快照 | 已覆盖状态、选中实例、缺失依赖、`enabled` |
| 管理面 | 已覆盖重新扫描、启停、依赖告警、未知插件 |
| UI 发布 | 已覆盖 `from` 可信来源、5 条错误码映射中的 4 条、无 publisher |
| Host 主动调用 | 已覆盖参数校验、事件顺序、nil 回调、期限、`plugin_down` |

未被测试固定的行为：

- 流水线三个错误码的线格式值。
- 重复 `host.proceed`。
- `instance_exists`、`instance_not_found`、`instance_closed` 的线格式值。
- `context_owner_mismatch` 的线格式值。
- 路由注销的时序效果。
- `plugin_disabled` 与 `capability_conflict`。
- 停止后依赖调用的错误码。
- 旧代际的应答与事件。
- `Host.Close` 之后的 `host_closed` 路径。
- UI 发布错误码映射里的 `bad_arguments`。
- 次要实例发布 UI 的行为。
