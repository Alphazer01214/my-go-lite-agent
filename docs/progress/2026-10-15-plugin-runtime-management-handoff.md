# 插件运行时管理交接

日期：2026-10-15  
分支：`webui`  
状态：设计讨论，尚未实现  
前置：[管理面开发日志](2026-10-15-management-plane-development-log.md)

## 1. 本次目的

本交接不冻结实现方案。目标是把插件启停、禁用持久化、重启和恢复拆成可讨论的决策。

当前 Plugins 页面只展示状态。下一步要决定用户能否改变状态，以及 Host 如何在进程崩溃和重启后恢复。

## 2. 当前代码基础

### 2.1 Host 生命周期

`internal/host/host.go` 已有：

- `disabled map[string]bool`；
- `plugins map[string]*proc`；
- `gen map[string]int`；
- `closed`；
- pending 死亡闭环。

`internal/host/proc.go` 已有：

- `proc.done`；
- `proc.healthy`；
- `markUnhealthy`；
- 进程退出回收。

`internal/host/constant.go` 已预留：

```text
SwitchFileName
HostMethodEnsurePlugins
HostMethodSetPluginEnabled
HostMethodPluginSwitch
PluginStateMounted
PluginStateAvailable
PluginStateDegraded
PluginStateDisabled
PluginStateMissing
```

当前没有读写禁用名单，也没有运行时启停、重启和恢复入口。

### 2.2 WebUI

Plugins 页面目前只读。Bootstrap 已提供插件状态、Capability 和 UI Mount。

Browser Call 仍禁止 `host.*`。生命周期管理属于 Host 平台管理，不应伪装成插件 Capability。

## 3. 需要统一的概念

运行时管理至少包含四个维度：

| 维度 | 示例 | 来源 |
|---|---|---|
| 期望状态 | enabled、disabled | 用户和持久化文件 |
| 进程状态 | running、exited、failed | Host |
| 健康状态 | healthy、unhealthy | Frame 和进程观察 |
| 依赖状态 | dependency mounted、missing | Plugin Graph |

当前 `PluginSnapshot.state` 把这些维度压缩为一个枚举。启停实现前需要决定是否继续压缩。

## 4. 状态模型

推荐保留 `enabled` 作为持久期望状态，`state` 作为派生观测状态。

推荐转换：

```text
available --enable--> mounted
mounted   --disable--> disabled
mounted   --exit----> degraded
degraded  --restart--> mounted
disabled  --enable--> mounted / degraded
```

派生规则：

| enabled | 进程 | 依赖 | state |
|---|---|---|---|
| false | 任意 | 任意 | disabled |
| true | healthy | healthy | mounted |
| true | unhealthy 或启动失败 | 任意 | degraded |
| true | 未启动且非计划启动 | 任意 | available |
| true | healthy | missing 或 disabled | degraded |
| 未发现 | 任意 | 被引用 | missing |

这样不会把“用户禁用”和“进程崩溃”混为一谈。

## 5. 启停语义

### 5.1 方案 A：启停等于持久 enabled

`enable` 立即启动并持久化。`disable` 立即停止并持久化。

优点：

- 只有一个用户意图维度；
- Host 重启后行为明确；
- UI 只需要一个开关。

缺点：

- 没有临时停止；
- 临时维护和长期禁用无法区分。

### 5.2 方案 B：增加临时 stopped

`enabled` 与 `running` 分离。用户可以临时停止，但 Host 重启后仍自动启动。

优点：

- 支持维护模式；
- 保留长期禁用语义。

缺点：

- UI 需要两个控制维度；
- 状态组合增加；
- 恢复规则复杂。

推荐先采用方案 A。临时停止可以后续通过维护模式增加。

## 6. 禁用名单持久化

### 6.1 方案 A：disabled 列表

```json
{
  "disabled": ["agent", "llm-openai"]
}
```

优点：

- 与 `SwitchFileName` 预留一致；
- 文件简单；
- 插件新增字段时无需迁移。

缺点：

- 只能表达禁用；
- 无法保存启动策略、重启策略和维护状态。

### 6.2 方案 B：期望状态映射

```json
{
  "plugins": {
    "agent": {
      "enabled": true
    },
    "llm-openai": {
      "enabled": false
    }
  }
}
```

优点：

- 可扩展；
- 可保存 restart policy、manual mode 等字段。

缺点：

- 现在引入未来字段属于投机设计；
- 文件迁移和兼容成本更高。

推荐先采用方案 A。确有第二个生命周期字段时再扩展格式。

## 7. 写入和启动顺序

推荐顺序：

```text
扫描 Manifest
→ 读取禁用名单
→ 计算 autostart + depends_on closure
→ 注册 Capability
→ 启动 enabled 插件
```

禁用名单必须在任何插件进程启动前生效。

写入规则：

- 临时文件写入同目录；
- 写入后替换目标文件；
- 写入失败时回滚内存状态；
- 禁用时先持久化，再停止进程；
- 启用时先持久化，再尝试启动。

启动失败不会撤销 `enabled=true`。状态显示 `degraded`，Host 下次启动继续尝试。

## 8. 运行时操作

### 8.1 Disable

1. 更新并保存 disabled 集合；
2. 关闭 stdin；
3. 等待进程正常退出；
4. 超时后强制结束；
5. 清理 pending；
6. 合成 `plugin_disabled` 或 `plugin_down`；
7. 更新快照。

### 8.2 Enable

1. 从 disabled 集合删除；
2. 保存文件；
3. 启动进程；
4. 启动成功返回 `mounted`；
5. 启动失败返回 `degraded`。

### 8.3 Restart

1. 停止旧进程；
2. 清理旧 pending；
3. 启动新进程；
4. 使用 `gen` 隔离旧进程退出事件。

是否需要 Restart 必须在 UI 中和 Enable/Disable 分开表达。推荐增加 Restart，因为崩溃后 enabled 仍为 true。

## 9. pending 语义

生命周期操作可能中断在途 Call。

推荐：

| 操作 | 在途 Call |
|---|---|
| Disable | 终止并返回 `plugin_disabled` |
| Restart | 终止并返回 `plugin_down` |
| Host Close | 返回 `server_closed` |
| 插件自行退出 | 返回 `plugin_down` |

事件和最终 Response 不重放。浏览器断开仍不取消业务 Call。

## 10. 依赖策略

### 10.1 不级联

禁用 `llm-openai` 时，不修改 `agent.enabled`。`agent` 因依赖不健康显示 `degraded`。

优点：

- 用户禁用意图不会扩散；
- 状态由依赖关系派生；
- 重新启用底层插件后 dependent 自动恢复。

缺点：

- dependent 仍在运行，可能持续产生失败调用。

### 10.2 级联

禁用依赖时同时禁用 dependent。

优点：

- 运行状态一致；
- 避免无意义调用。

缺点：

- 会修改其他插件的持久意图；
- 重新启用需要逆向恢复；
- 多级依赖和循环依赖规则复杂。

### 10.3 阻止

依赖不健康时禁止启用 dependent。

优点：

- 不启动无效插件。

缺点：

- 启用顺序和错误提示复杂；
- 需要事务式依赖解析。

推荐先采用不级联，并把 dependent 标记为 `degraded`。这是最小行为变化。

## 11. 崩溃恢复

### 11.1 手动恢复

崩溃后保持 `degraded`。用户点击 Restart 后尝试恢复。

优点：

- 不会形成崩溃循环；
- 故障可见；
- 行为可预测。

缺点：

- 需要人工介入。

### 11.2 有限自动重启

采用退避和最大次数，例如 3 次。

优点：

- 临时故障自动恢复。

缺点：

- 需要保存 restart count；
- 日志和状态复杂；
- 配置错误仍可能反复启动。

### 11.3 Supervisor

由独立 supervisor 管理插件生命周期。

优点：

- 故障隔离；
- 支持复杂恢复策略。

缺点：

- 引入第二运行时；
- 与当前 Host 薄内核冲突；
- 会扩大爆炸半径。

推荐 v1 只支持手动 Restart。Host 启动时恢复 enabled 插件，不做运行中自动重启。

## 12. 管理接口

### 12.1 Host seam

推荐新增窄接口：

```go
type PluginLifecycle interface {
    SetPluginEnabled(name string, enabled bool) (PluginSnapshot, error)
    RestartPlugin(name string) (PluginSnapshot, error)
}
```

`Host` 实现该接口。WebUI 不读取 `proc`、stdin、cmd 和 gen。

启动入口需要知道插件目录。推荐扩展：

```go
type RunOptions struct {
    PluginDir string
    LogFunc   func(string)
}

func Run(discoveries plugin.Discoveries, options RunOptions) (*Host, error)
```

### 12.2 HTTP seam

生命周期是 Host 平台管理，不使用 Browser Call 的 `host.*` 路由。

候选接口：

```text
POST /bridge/plugins/{plugin}/enabled
POST /bridge/plugins/{plugin}/restart
```

启用和禁用请求体：

```json
{
  "enabled": false
}
```

返回 Plugin Snapshot。请求必须校验插件名、HTTP method 和 JSON 尾随数据。

## 13. UI 行为

Plugins 页面建议增加：

- enabled/disabled 开关；
- Restart 图标按钮；
- 请求期间禁用当前行控件；
- 行内错误；
- 成功后使用 Snapshot 更新状态。

Disable 是可逆的本地操作，不需要额外确认。关闭面板不取消生命周期请求。

## 14. 测试矩阵

### 14.1 持久化

- 读取空文件；
- 读取 disabled 列表；
- 去重和排序；
- JSON 损坏；
- 写入失败；
- 权限和替换结果。

### 14.2 生命周期

- Host 启动跳过 disabled；
- Disable 立即停止；
- Enable 立即启动；
- 启动失败保持 enabled；
- Restart 创建新 gen；
- 旧 gen 退出不影响新进程；
- pending 死亡闭环；
- 依赖 disabled 时 dependent 为 degraded；
- 重新启用后 dependent 恢复 mounted。

### 14.3 WebUI

- 开关状态；
- Restart 可用性；
- 请求失败保留原状态；
- 未知插件；
- 非法 body；
- 响应 Snapshot 更新。

## 15. 建议切片

### 切片 A：生命周期模型和持久化

只实现 `disabled` 文件、Host 状态转换和单元测试。不接 HTTP 和 UI。

### 切片 B：运行时 Enable/Disable

实现进程停止、启动、pending 闭环和 Snapshot 更新。

### 切片 C：Restart 与依赖派生

实现 gen 隔离、手动 Restart 和 dependent degraded。

### 切片 D：管理 HTTP 和 Plugins UI

接入管理接口、开关、Restart 按钮和浏览器测试。

## 16. 待确认决策

开始编码前需要确认：

1. 启停是否统一为持久 `enabled/disabled`，不增加临时 `stopped`；
2. 禁用是否不做依赖级联，只把 dependent 标记为 `degraded`；
3. 崩溃后是否只支持手动 Restart，不做自动重启；
4. 禁用名单是否先采用 `.plugin-switch.json` 的 disabled 列表；
5. 管理接口是否使用 `/bridge/plugins/{plugin}/enabled` 和 `/bridge/plugins/{plugin}/restart`。

推荐答案依次为：是、是、是、是、是。
