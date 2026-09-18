# serve — Host L0 内核

规范名词：[Host](../../CONTEXT.md)、[Frame](../../CONTEXT.md)、[hostFaces](../../CONTEXT.md)、[Ensure Mount](../../CONTEXT.md)、[Render Medium](../../CONTEXT.md)。相关 ADR：0001、0014、0016、0017、0023、0026、0027、0030。

## 职责

`serve` 是 ADR-0030 之后的 **Host**：持有进程外插件生命周期，**按插件名**转发 Frame。

**不做**：按 Capability 路由、合并 tools、解析领域 payload、拥有 Agent Loop 锁、实现 session/turn HTTP 面。

`provides`/`consumes` 仅作声明与观测（degraded、Plugin Graph），不是路由表。

## 文件

| 文件 | 角色 |
|------|------|
| `serve.go` | Server 字段、Start、CallByFace、CallByPlugin、catalog |
| `router.go` | Frame 分发、complete、PanelOp 校验、EnsurePlugins、deferred agent.* |
| `transport.go` | 进程 I/O、launch/readLoop/ensureAlive、Host 主调 call* |
| `registry.go` | RegistrySnapshot、registerProvides、reconcileConsumes |
| `context.go` | Deferred ADR-0002：AgentRequest/AgentInject、callByCapOwner |
| `session.go` | pluginsdk 类型别名 + deferred 辅助 |
| `medium.go` | Presentation/UI/commands 常量、Cards/recordCard |
| `events.go` | Subscribe/publish、panel ring、RegisterApproval/askApproval |
| `render.go` | Kind 别名、TruncateRunes |
| `debug.go` | SetDebug、stderr Frame 日志 |
| `job_windows.go` / `job_other.go` | Windows Job Object / no-op |

## 导出表面（摘要）

| 类别 | 符号 |
|------|------|
| 常量 | `DefaultCallTimeout`（30s）、`DefaultShutdownGrace`（2s）、`HostCap`、Presentation/UI/Commands 相关 |
| 类型 | `Server`、`CallResult`、`RegistrySnapshot`、`EnsurePluginsResult`、`Event`、`Subscriber` |
| 函数 | `Start`、`CallByFace`、`Registry`、`RegisterApproval`、`MarshalPayload`、`TruncateRunes`、`SetDebug` |
| 方法 | `Close`、`CallByPlugin`、`EnsurePlugins`、`SetCatalog`、`SetPluginsDir`、`MountedPluginNames`、`DegradedNames`、`Subscribe`、`Cards`、`Panels`、`AgentRequest`、`AgentInject`、`SetPluginEnabled`、`DisabledPluginNames`、`CallHost` |

## 核心数据结构

- **Server**：`plugins map[name]*proc`、`provides`（观测）、`catalog`/`pluginsDir`、`degraded`、`pending`、`cards`/`panels` ring、`subs`、`job`、`approvals`
- **proc**：found、cmd、stdin、healthy、timeout、gen（重启代数）
- **wait**：waitHost \| waitPlugin；origID；ch；events
- **Event**：Topic ∈ `presentation|status|stream|panel`

## Frame 路由（L0）

```text
plugin stdout → readLoop → handleFromPlugin
  req  → routeRequest
           host_closed?
           cap/to == host → ensurePlugins | plugins
           deferred: cap=agent method∈{request,inject,confirm}
           empty to → to_required
           to==from → route_failed
           else forwardTo(to)：id 改写 fwd-N，超时定时器
  res  → complete：waitHost 送 channel；waitPlugin 恢复 origID 回写调用方
  evt  → collectEvent：Presentation/panel 特殊处理；有 id 则挂到 wait
```

**Host 主调**：`callOnce` 分配 `host-N`；`plugin_down` 时 `ensureAlive` 重启并重试一次。

## 生命周期

1. **Start**：创建 Job → registerProvides（非 tools 能力唯一属主；冲突警告）→ UI-only 记入 mountedUI → 逐个 launch → reconcileConsumes → **总是返回 *Server**（软失败）
2. **launch**：stdin/stdout pipe；stderr 透传；Job 绑定；timeoutMs 或默认；bump gen；`go readLoop`
3. **不健康**：`markUnhealthy` 失败 pending（`plugin_down`）→ reconcile；下次调用 `ensureAlive` 杀树重拉
4. **Close**：closed；EOF 全部 stdin；grace 2s；killTree；job.close

## hostFaces

`CallByFace(s, plugin, face, method, payload)`：Manifest 必须声明该 face，否则拒绝。Frame `Cap=face`。

用于：`config.reload`（/refresh）、`commands.call`、`ui.action`、Settings config.schema/get。Faces 进 `RegistrySnapshot.Faces` 与 Plugin Graph。

## `host.ensurePlugins`

Payload `{names:[...]}` → 重扫 catalog（若设了 pluginsDir）→ `assembly.ResolveClosure` → 已挂载/UI → Mounted；否则 launch。幂等（ADR-0023）。Agent Scheme `dependsPlugins` 由此拉起工具插件。

## 插件启用开关（ADR-0032）

- 持久化：`<pluginsDir>/.plugin-switch.json` `{"disabled":[...]}`
- Host 方法：`host.setPluginEnabled` `{name, enabled}`、`host.pluginSwitch`
- 关：立即卸载进程/UI，Autostart / EnsurePlugins / ensureAlive / launch 均跳过
- `ensurePlugins` 响应含 `disabled[]`；RegistrySnapshot / Plugin Graph 含 disabled

## Deferred `agent.*`（ADR-0030）

仅 `request` / `inject` / `confirm` 仍是 Host 特例：

- **request** → AgentRequest：session.derive 比对不变量（未记日志则拒绝）
- **inject** → 规范化后 session.append
- **confirm** → askApproval（并行问所有 Medium 面，先到先得；无人应答 → deny）

**禁止扩大该分支**；重开需新决策。

## 软失败（ADR-0017）

单插件失败不杀进程；能力冲突不注册即可；死插件 → pending 失败 + provides 撤回 + 传递 degraded；Panel 非法 → status 警告；裸 Host 无插件仍可起。

## 测试

| 测试 | 覆盖 |
|------|------|
| panel_test | set/clear、前缀、非法 tag/slot、props |
| reconcile_test | degrade、provides 撤回、恢复、gen++ |
| render_test | CJK 截断、RenderIntent JSON（sessionId 恒在） |
| debug_test | SetDebug、payload 截断 |
| job_windows_test | Job 关闭杀子进程 |

缺口：forward 超时、EnsurePlugins、AgentRequest 不变量、CallByFace 门禁——由 cmd/web 集成测试间接覆盖。

## 开发约定（红线）

1. 新 Host 功能必须 L0：生命周期、挂载、不透明转发、通用事件/应答槽。
2. 禁止 `switch` 业务能力名；禁止解析 payload 字段级结构体做业务。
3. 禁止出现 `plugins/` 下目录名字面量（测试/构建脚本除外）。
4. 不要把 tools 合并、Loop 锁、session 不变量搬回 Host。
5. Presentation 契约方法特判属公共展示面，不是领域路由——保持最小集。
