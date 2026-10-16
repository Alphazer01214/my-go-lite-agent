# CONTEXT

本文统一 LiteAgent 的平台术语。插件业务术语由各插件文档定义。

## 1. 运行时概念

### Host

Host 是薄平台内核。它负责发现插件、启动进程、路由 Frame、维护调用闭环和提供 WebUI 平台壳。

Avoid：server、runtime（指 Host 时）。

### Plugin

Plugin 是进程外可执行单元。Plugin 通过 stdio 与 Host 交换 Frame。插件不得直连其他插件。

### Plugin Instance

Plugin Instance 是一个 Plugin 的进程激活身份。同一个 Plugin 可以同时存在多个 Plugin Instance。Host 分配 `instance_id`，并用它隔离进程、代号和 pending。

### Execution Context

Execution Context 是 Host 管理的通用调度范围。它记录根、父级和 Capability 绑定。Host 不解释会话、子代理等插件业务术语。

### Config Revision

Config Revision 是配置快照的不透明版本标识。Host 只传递和追踪 Config Revision，不读取插件配置内容。Plugin Instance 在创建时固定配置代。Execution Context 通过实例绑定间接固定配置。

### Capability

Capability 是业务能力名。路由键是 `(capability, method)`。插件名不参与路由。

### Requires

Requires 表示弱依赖能力。能力缺失时插件仍可启动，并在调用时返回可恢复错误。

### DependsOn

DependsOn 表示硬依赖插件名。Host 用它安排启动顺序。DependsOn 不参与 Frame 路由。

### Identity

插件身份是插件进程声明的插件名。身份用于日志、溯源和依赖图，不用于业务寻址。

## 2. 通信概念

### Frame

Frame 是 Host 与 Plugin 之间的一条完整消息。线格式是 4 字节大端长度加 UTF-8 JSON。

Avoid：message（指 Frame 时）、packet。

### Request

Request 是 `type=req` 的 Frame。Request 要求对端执行一个 `capability.method`。Request 可以携带 `context_id`，用于选择 Plugin Instance。

### Call

Call 是 pluginsdk 发起的一次同步 Request。Call 等待同 id 的 Response。

### Response

Response 是 `type=res` 的 Frame。Response 与 Request 使用同一个 id 闭环。

### Event

Event 是 `type=evt` 的 Frame。Event 不建立 Request/Response 闭环。带 id 的 Event 属于一次在途 Call。无 id 的 Event 是广播事件。归因 Event 保留所属执行上下文。

## 3. Manifest 概念

### Manifest

Manifest 是插件包的静态注册面。它描述身份、能力、依赖、UI 和 Settings Schema。Manifest 不保存运行状态和真实密钥。

## 4. WebUI 概念

### Component

Component 是插件拥有的原生 Custom Element。Component 通过 `setData` 接收状态，通过 `runCommand` 响应瞬时指令。

### Mount

Mount 是 Component 的一个实例位置。实例身份是 `plugin_name + mount_id`。`mount_id` 只定位实例，不参与业务路由。

### Slot

Slot 是 Host Shell 的布局位置。v1 Slot 为 `top`、`left`、`center`、`right` 和 `bottom`。

### State

State 决定 Component 当前是什么。State 的业务含义由插件定义。

### Command

Command 决定 Component 此刻做什么。滚动、聚焦和临时动画属于 Command。Command 不是插件持久化记录。

### Web Bridge

Web Bridge 是 Host 对浏览器的传输适配器。它封装 Call、Response、Event、State 和 Command。Web Bridge 不理解业务字段。

## 5. 业务术语

插件的存储单元、领域对象、消息、任务、工具和 UI View 都属于插件契约。平台文档使用 `payload`、`state` 等中性词，不代替插件定义业务术语。
