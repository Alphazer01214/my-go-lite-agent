# Host 运行期基础设施开发日志

> 状态：当前实现
> 说明：本文的三条并发规则是唯一的书面记录，代码在 `internal/host/transport.go`。「Frame v1」的说法已过时，线协议版本是 2。

日期：2026-10-15  
分支：`webui`  
范围：Plugin Instance、Execution Context、上下文调用传播  
状态：基础闭环已完成。调度策略按实际日志按需实现。

## 1. 本次结论

Host 已具备多 Plugin Instance 和通用执行上下文的基础能力。

本次完成：

1. Plugin Instance 注册、创建、关闭和重启；
2. Execution Context 创建、继承、绑定和关闭；
3. `context_id` 在 Request、Response 和归因 Event 中传播；
4. Capability 通过执行上下文选择目标 Plugin Instance；
5. pluginsdk 提供上下文 Call 和 Event 入口；
6. 旧进程代号的 Frame 在路由入口被拒绝。

Host 不解释会话和子代理。两者仍是插件业务概念。

## 2. 设计边界

### 2.1 Plugin Instance

Plugin Instance 是一个 Plugin 的进程激活身份。

当前实现中：

- 默认实例 id 等于插件名；
- 同一 Plugin 可以创建多个实例；
- 每个实例保存独立进程和代号；
- `from` 继续表示 Plugin 身份；
- `instance_id` 不进入 `from`。

实例快照包含 `instance_id`、`plugin_name`、`config_revision`、`state` 和 `generation`。

### 2.2 Execution Context

Execution Context 是 Host 管理的通用调度范围。

它记录：

- `id`；
- `owner`；
- `root_id`；
- `parent_id`；
- Capability 到 Plugin Instance 的绑定；
- 参与者实例。

子上下文继承父上下文的 Capability 绑定。调用方只能显式覆盖部分绑定。

### 2.3 配置代

`config_revision` 是 Plugin Instance 的不透明标识。Host 不读取插件配置内容。

Plugin Instance 创建时固定配置代。Execution Context 通过 Capability 绑定间接固定配置。配置快照的生成、解释和持久化仍由插件负责。

## 3. 协议修改

`protocol.Frame` 新增可选字段：

```text
context_id
```

该字段是 Frame v1 的向后兼容扩展。

行为规则：

- 无 `context_id` 的调用保持原路由；
- 有 `context_id` 时，Host 校验调用方是参与者；
- Capability 绑定决定目标 Plugin Instance；
- 无绑定时使用默认 Plugin Instance；
- Response 和归因 Event 保留执行上下文；
- 目标实例加入上下文后，可以继续传播该上下文。

## 4. Host 方法

本次保留的 Host 方法：

| 方法 | 用途 |
|---|---|
| `host.context.create` | 创建执行上下文 |
| `host.context.bind` | 修改 Capability 实例绑定 |
| `host.instance.create` | 创建 Plugin Instance |
| `host.instance.close` | 关闭 Plugin Instance |
| `host.instance.restart` | 重启 Plugin Instance |

`host.context.close` 同步关闭执行上下文子树。Host 终止相关 pending，并删除已关闭上下文，避免长期运行时积累空上下文。

## 5. 代码修改

主要落点：

```text
protocol/frame.go
protocol/errors.go
internal/host/runtime.go
internal/host/host.go
internal/host/proc.go
internal/host/transport.go
internal/host/call.go
internal/host/snapshot.go
pluginsdk/plugin.go
```

`Host.plugins` 现在按 `instance_id` 索引进程。默认实例仍使用插件名，因此既有单实例行为保持不变。

`wait` 同时保存调用方实例、目标实例、插件身份和执行上下文。Host 对外仍只暴露插件身份。

## 6. pluginsdk 修改

新增入口：

```text
CallInContext
CallWithCallbackInContext
EmitInContext
EmitWithIDInContext
```

`Request` 和 `Event` 新增 `ContextID`。插件收到调用后，可以把同一执行上下文继续传播给下一跳。

现有 `Call`、`CallWithCallback`、`Emit` 和 `EmitWithID` 保持无上下文语义。

## 7. 并发和故障处理

本次固定了三条规则：

1. Frame 只能来自当前 `generation`；
2. 旧进程代号的 Frame 在 `routeFrame` 入口丢弃；
3. 写 stdin 时不再持有 Host 全局锁。

Plugin Instance 重启会终止该实例的 pending 调用。其他实例不受影响。

执行上下文关闭会返回 `context_closed`。插件进程退出仍返回 `plugin_down`。

Host 现在记录实例创建、退出、重启、上下文创建、绑定和关闭。日志还记录旧代号 Frame 丢弃和 pending 失败数量。后续调度设计以这些日志为依据。

## 8. 验证记录

命令：

```text
go test ./...
go test -race ./internal/host ./internal/webui ./pluginsdk
go vet ./internal/host ./protocol
go build ./cmd/liteagent-server
git diff --check
```

覆盖场景：

- 上下文继承 Capability 绑定；
- 上下文调用进入绑定实例；
- 默认实例不接收绑定实例调用；
- 目标实例加入上下文并可继续传播；
- 非参与者不能复用上下文；
- 关闭父上下文会关闭子上下文并终止 pending；
- Host 上下文方法完整闭环；
- 实例快照包含配置代和代号；
- 旧 `generation` 的 Frame 被拒绝；
- pluginsdk 保留 Request、Event 和出站调用的 `context_id`。

## 9. 当前限制

本次没有实现：

- 调度队列、优先级和资源配额；
- 在线迁移 Plugin Instance；
- Plugin Instance 崩溃后的透明恢复；
- 配置快照生成和 Secret 管理；
- WebUI 的实例和上下文管理页面；
- 浏览器 Call 的 `context_id`；
- Plugin Instance 操作的属主授权；
- 多浏览器运行时状态同步。

`host.instance.close` 和 `host.instance.restart` 当前是平台管理能力。正式开放给不可信插件前，需要增加属主和授权检查。

## 10. 完备性结论

对于当前本机、单用户、可信插件的使用方式，Host 的基础闭环已经完整：

- Plugin 身份与 Plugin Instance 身份分离；
- 多实例创建、关闭和重启；
- Capability 路由和 Context 实例绑定；
- Request、Response、Event 传播 `context_id`；
- pending 按实例和上下文闭环；
- 旧 `generation` 隔离；
- 基础生命周期日志。

Host 暂时不需要通用调度器。当前直接转发模型可以先投入实际使用。

它尚不适合不可信插件、多用户或高并发场景。实例属主授权、调用超时和资源隔离仍然缺失。

## 11. 下一步

下一阶段以运行和诊断为主：

1. 在实际插件组合中收集 Host 日志；
2. 记录慢调用、实例退出和 pending 无法闭环的情况；
3. 根据日志定位真实争用点；
4. 只为已观察到的问题增加最小调度机制；
5. 暂不预建队列、优先级和配额系统。

运行时管理交接见 [插件运行时管理交接](2026-10-15-plugin-runtime-management-handoff.md)。
