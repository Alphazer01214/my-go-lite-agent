# sandbox 裁决实现记录

> 状态：当前实现
> 说明：本文记录 sandbox 首批（三态裁决、规则集、审计）的实现状态与决策留底。
> 插件契约见 [plugins/sandbox/communication.md](../../../plugins/sandbox/communication.md)。
> 目标契约见 [docs/design/plugins/08-sandbox.md](../../design/plugins/08-sandbox.md)。

本文记录 `sandbox` 插件首批的实现结果。它是第一批落地的插件，接在
[消费链组合方案](../../design/composition-model-context-tools-sandbox.md) 之后。

## 1. 交付范围

| 项 | 状态 | 内容 |
|---|---|---|
| 三态裁决 | 已实现 | `allow`、`modify`、`deny` |
| 接缝 | 已实现 | `tools.call`、`file.write`、`file.replace`、`file.patch`、`shell.execute` |
| 规则集 | 已实现 | 规则匹配、动作优先级、只升不降、持久化 |
| 审计 | 已实现 | `sandbox_decision` 会话事实 |
| 插件方法 | 已实现 | `sandbox.check`、`sandbox.list_rules`、`sandbox.set_rule`、`sandbox.settings.get/save` |
| 确认流 | 未实现 | `confirm` 动作、确认令牌、两阶段续传 |
| 确认面板 | 未实现 | 右槽组件 |
| Prompt 贡献 | 未实现 | 动态轨 `policy` 节 |

## 2. 逐点决策

这一轮的四个决策点都在动手前确认过。结论如下。

### 2.1 首批范围

首批交付三态裁决、规则集与审计，一次覆盖 `tools.call` 与四个直连高危接缝。
确认流、确认面板与 `policy` 轨节不在首批。

### 2.2 拒绝的返回形态

拒绝返回工具结果包络，`is_error` 为 `true`，`meta.code` 是 `sandbox_denied`。
它不是协议错误。

原因：模型要能读到拒绝原因。协议错误会被 agent 转成 `tool error [code]`，
结构化字段只留在 `meta` 里，包络能让原因与代码同时到达模型。
`confirm` 落地后沿用同一形态。

### 2.3 意图区字段

sandbox 沿用现有 `tools.call` 载荷的扁平字段，不引入 `meta` 包装层。

原因：`meta` 意图区要改 agent、tools 与 sandbox 三方契约。
裁决只需要 `session_id`、`workspace`、`call_id`，扁平字段已经够用。
代价是审计条目里 `turn_id` 与 `step_id` 是空串。

### 2.4 风险档位

已定的目标映射：

| 范围 | 目标动作 | 首批动作 |
|---:|---|---|
| 0 到 49 | `allow` | `allow` |
| 50 到 79 | `confirm` | `deny` |
| 80 到 100 | `confirm` | `deny` |

未知工具的兜底风险是 70，落在确认档。

首批没有 `confirm`，所以确认档按 `deny` 执行。
这让写文件与命令执行在默认策略下被拒。
因此 sandbox 的 `autostart` 是 `false`：默认部署不装裁决，启用的部署才有牙。
确认流落地后把两档改成 `confirm`，并把 `autostart` 改回 `true`。

## 3. 实现要点

### 3.1 消费链

sandbox 的 `priority` 是 500，所以它在 `tools` 之前。`tools.call` 的链序是
sandbox → tools。tools 分发时对目标能力的调用重新进链，直连高危接缝也过裁决。

Consumer 注册不抢 Provider 路由，所以 sandbox 与 tools 可以注册同一个键。

### 3.2 风险解析

sandbox 调 `tools.list` 读注册表里的 `risk`，结果缓存 5 秒。
直连接缝按目标能力与目标方法反查工具名。
`tools.list` 失败时全部按未知风险 70 判定。

### 3.3 规则只升不降

范围包含用保守近似判断：字面量只覆盖相同字面量，含通配符的一侧按更宽处理，
判不出来的关系按不覆盖处理。更严的动作不受范围比较约束。

### 3.4 审计

裁决写一条 `sandbox_decision` 会话事实。事实类型不带 `message`，不进模型上下文。
`session` 缺席时退化为进程内的有界环形缓冲。

## 4. 执行证据

在 `dist/` 产物上跑过真实模型。

- 模型请求写文件，`file_write` 被拒（风险 80），模型如实报告了拒绝原因。
- 模型改用命令执行绕过，`shell_execute` 被拒（风险 90），模型没有伪造结果。
- 规则生效：`sandbox.set_rule` 加一条 `deny` 规则后，下一次调用按规则拒绝。
- 规则持久化：插件重启后规则仍在。
- 弱化被拒：新增一条更宽的 `allow` 规则返回 `sandbox_rule_invalid`。
- 审计落盘：会话事实里有 `sandbox_decision`，带风险值与参数摘要。

没有 `policy` 轨节时，模型在被拒后会换一条路再试。
这是下一批要补的：提前告知策略，模型就不浪费轮次。

## 5. 已知边界

| 项 | 说明 |
|---|---|
| 浏览器发起的调用 | Host 发起的调用直接解析路由属主，不装配消费链。浏览器发起的 `tools.call` 不经 `tools.call` 这一跳。直连高危接缝仍过裁决 |
| 确认档降级 | 50 以上按 `deny` 执行，没有人工决策环节 |
| 规则包含关系 | 保守近似，不是完备的包含判定 |
| 审计保留 | 会话事实日志不由本插件裁剪，`audit_retention_days` 只约束内存缓冲 |
| 删除规则 | 目标契约没有删除方法，当前实现用 `sandbox.set_rule` 的 `delete` 字段 |
