# 2026-10-20 全量测试修复方案

> 状态：当前实现
> 说明：P0 死锁链的推理、U8 与取消语义与慢连接的方案表是唯一记录。行号已过时，决策仍有效。

本文研究 2026-10-20 全量测试发现的问题。

Phase 1 的写锁与超时修复已实施。
实现、锁语义与实测见 [2026-10-20-p0-write-lock-fix.md](2026-10-20-p0-write-lock-fix.md)。

Phase 2 与 P1 也已实施。
实现与实测见 [2026-10-20-phase2-p1-fix.md](2026-10-20-phase2-p1-fix.md)。

P2 的取消语义、慢连接恢复、工具错误文案与错误清理也已实施。
实现与实测见 [2026-10-20-p2-fix.md](2026-10-20-p2-fix.md)。
剩余：Host writer goroutine、大 State 预算、W1 模型行为。

测试报告见 [2026-10-20-full-test-report.md](2026-10-20-full-test-report.md)。

## 1. 修复顺序

| 阶段 | 内容 | 目标 |
|---|---|---|
| Phase 1 | SDK 写锁与超时热修 | 解除 P0 死锁 |
| Phase 2 | 流式发布解耦与合并 | 降低 UI 发布压力 |
| Phase 3 | P1 可见性、模型、工作区、resync | 恢复主路径一致性 |
| Phase 4 | P2 取消语义、慢连接、错误文案 | 补齐边界体验 |

先做 Phase 1。
Phase 2 与 Phase 3 可以并行。
Phase 4 中的契约变更需要单独确认。

## 2. P0 死锁

### 2.1 阻塞链

问题不是单纯的 CPU 或内存。
进程没有打满，插件也没有退出。
阻塞点在插件 SDK 与 Host 的同步写回。

现场日志与代码对应的阻塞链：

1. session 的 `stream.update` 调用 `ui.SendCommand`。
2. `SendCommand` 通过 `p.Call` 等待 Host 的 `ui.command` 响应。
3. 插件写 stdout 时，`writeFrame` 持有 `p.mu`。
4. Host 读 session 的 stdout，处理响应。
5. Host 把响应写回 session 的 stdin。
6. session 的 `Listen` 读到响应后，`complete` 需要 `p.mu`。
7. `p.mu` 被第 3 步的阻塞写持有。
8. session 不再读 stdin，Host 的写回也阻塞。
9. 两边互相等待，形成死锁。

关键代码：

- `pluginsdk/plugin.go:655`：`writeFrame` 持 `p.mu` 写 stdout。
- `pluginsdk/plugin.go:450`：`waitResult` 等待响应。
- `pluginsdk/plugin.go:294`：`dispatch` 结束后写 response。
- `internal/host/transport.go:517`：Host 按插件同步读 stdout。
- `internal/host/transport.go:328`：Host 读循环里同步写回调用方。
- `internal/host/transport.go:499`：Host 写 stdin 时持 `plg.mu`。

`uiPublishTimeout` 不能救场。
超时 goroutine 也调用 `writeFrame`。
它同样需要 `p.mu`，所以超时本身也会卡住。

### 2.2 修复 A：拆分写锁

这是最小热修。

改动：

- `Plugin` 增加独立 `writeMu sync.Mutex`。
- `writeFrame`、`Emit`、`EmitWithID`、`writeCancel` 改用 `writeMu`。
- `p.mu` 只保护 `handlers`、`pending`、`inbound` 等状态。
- Host 的 `proc.mu` 当前只用于写序列化，本次不改。
- Phase 2 可以改为每插件 writer goroutine。

效果：

- `Listen` 的 `complete` 不再等待写 stdout。
- session 能持续消费 stdin。
- Host 的写回可以完成。
- 死锁链被切断。

风险：

- 写顺序需要由 `writeMu` 保证。
- 不能持有 `p.mu` 调用写路径。
- `Emit` 与 response 的跨类型顺序仍需测试。

### 2.3 修复 B：让超时本地生效

即使拆分写锁，仍不建议依赖 cancel 帧解超时。

改动：

- `waitResult` 的循环增加 `case <-ctx.Done()`。
- 超时时删除 `pending`，直接返回 `call_deadline_exceeded`。
- cancel 帧改为异步尽力发送，不阻塞返回值。
- 保留现有 `markOutboundCancel` 语义。

效果：

- UI 发布最多等一个超时窗口。
- session handler 不会永久占住一个 goroutine。
- 超时后迟到的响应被安全丢弃。

风险：

- 超时与迟到响应之间要保证幂等。
- `complete` 对未知 id 已经会丢弃，风险可控。

### 2.4 修复 C：流式发布解耦

这是性能与稳定性的关键。

现状：

- 每个 `stream.update` 都同步调用一次 `host.ui.command`。
- 每次调用都跨进程往返。
- Host 每个连接只有 64 帧队列。
- 队列满后丢帧，不通知页面。

建议：

- session 插件增加 latest-value outbox。
- 每个 session 只保留最新 stream 快照。
- 后台 goroutine 以 30～60 Hz 刷新。
- UI 命令使用短超时。
- 发送失败只标记需要 resync，不阻塞 handler。
- `stream.update` 只更新内存状态并入队，立即返回。

可选结构：

```text
type streamOutbox struct {
    mu      sync.Mutex
    latest  map[string]streamDeltaUI
    wake    chan struct{}
}
```

约束：

- 同一 session 的 delta 顺序不能乱。
- 合并只允许丢中间帧，不能丢终态。
- 终态 `completed`、`cancelled`、`error` 必须立即发送。
- 失败后由 UI 的 seq 检查触发 resync。

### 2.5 Phase 1 验收

新增单元测试：

- stdout 阻塞时，`Listen` 仍能处理 response。
- 超时后 `waitResult` 返回，不依赖 cancel 写成功。
- Host 写回阻塞时，其他能力仍可响应。

集成回归：

- 运行 W1，再执行 30 次 200 ms 刷新。
- `session.workspace.list` p95 小于 100 ms。
- `session.state` 在 1 秒内返回。
- Turn 的 Fact 持续增长。
- 页面 5 秒内恢复选中会话。
- 活动页面无 `queue full`。
- 最后执行 S4 与 S7。

Phase 1 结果：

- 30 次刷新叠加流式 Turn 通过。
- 最后页面 1 ms 内就绪。
- 79 次 `session.state` 全部返回。
- 最大 State 191 KB，最大耗时 36 ms。
- Turn 最终 `completed`。
- 无 `queue full`。
- D2 与 10 次刷新也通过。
- 详细数据见 [写锁修复文档](2026-10-20-p0-write-lock-fix.md)。

## 3. P1 修复

### 3.1 失败 Turn 不可见

已确认根因：

- `renderLayout` 只在过程消息前插入折叠头。
- 过程消息只包括 assistant 与 tool。
- 只有用户消息的失败轮没有折叠头。
- 因此状态和错误都不可见。

代码位置：

- `plugins/session/ui/main.js:2224`。
- `plugins/session/ui/main.js:2107`。

改动：

- `chatTurnInfoUI` 增加 `error` 字段。
- `chatStateInternal` 在 `turn_end` 时写入 `meta.error`。
- 前端为每个 Turn 生成状态行。
- 没有过程行时，状态行放在该轮最后一条消息之后。
- 状态行显示“第 N 轮 · 出错/已取消”和错误摘要。
- 有过程行时，折叠头继续使用现有样式。
- 成功 resync 后清空旧错误提示。

错误文案：

- Host 的 `pipeline_loop` 不应直接暴露。
- 当 provider 不在 active 状态时，返回 `plugin_not_ready`。
- 消息包含缺失的插件名或能力名。
- UI 再不济也要把错误码和消息分开显示。

代码位置：

- `internal/host/pipeline.go:128`。
- `internal/host/transport.go:77`。
- `plugins/agent/turn.go:400`。

### 3.2 默认模型不生效

已确认根因：

- `applyCatalog` 只在当前模型不在目录中时切换。
- 保存默认模型后，旧模型仍在目录中。
- 所以 `selectedModel` 不更新。

代码位置：

- `plugins/session/ui/main.js:1245`。
- `plugins/session/ui/main.js:1257`。

建议规则：

- 记住上一次默认模型。
- 如果用户没有手动改过下拉，跟随新默认。
- 如果用户手动选过其他模型，保留用户选择。
- 改变默认模型时立即刷新下拉值。

可测试用例：

- 默认 A，刷新目录后默认 B，选择应变为 B。
- 用户选 C 后，默认变为 B，选择应保持 C。
- 用户选 C 后，目录移除 C，选择应回退到默认。

### 3.3 工作区不一致

先确认产品语义。

当前代码是刻意保留跨工作区 pane。
注释明确说明：
“registry workspace switch must not flip the view”。
相关位置：

- `plugins/session/ui_state.go:569`。
- `plugins/session/ui_state.go:994`。

这与 PRESSURE U8 的“无跨工作区会话”冲突。

方案选项：

| 方案 | 行为 | 优点 | 缺点 |
|---|---|---|---|
| A | 切换工作区时清除外来选中，优先选新工作区最近会话 | 列表、上下文、工作区一致 | 当前阅读被打断 |
| B | 保留外来 pane，但显示工作区警告；点击外来 tab 自动切回 | 不打断阅读 | 状态仍可能被误读 |
| C | 关闭所有不属于新工作区的 tab | 最简单一致 | 丢失打开标签 |

推荐 A。
保留跨区标签，但选中会话必须属于当前工作区。
点击外来 tab 时，`session.select` 已经会切换工作区。
切换工作区时，若没有目标会话，显示空状态。

需要改动：

- `session.workspace.select` 后解析新工作区的选中会话。
- `publishPanes` 不再默认包含外来 on-screen pane。
- 增加 U8 回归测试。

### 3.4 插件重启后页面 stale

已确认根因：

- 管理接口返回新的 Bootstrap。
- Shell 只调用 `applyBootstrap(payload)`。
- 没有传 `resync: true`。
- 已挂载组件不会重新读取状态。

改动：

- `runPluginAction` 调用 `applyBootstrap(payload, { resync: true })`。
- `rescanPlugins` 同样处理。
- 或者比较插件 generation，只 resync 受影响的 Mount。

代码位置：

- `internal/webui/static/app.js:225`。
- `internal/webui/static/app.js:245`。
- `internal/webui/static/loader.mjs:122`。

补充：

- session 插件启动后可能没有选中会话。
- `resync` 需要重新读取 history 和 state。
- 成功 resync 后清理旧错误提示。
- 增加管理动作后的 UI 测试。

## 4. P2 修复

### 4.1 取消语义

需要产品决策。

选项：

| 选项 | 行为 |
|---|---|
| A | 只加 UI“已取消”标记，上下文保持现状 |
| B | 取消时写一条 host 事实，提示模型不要继续未完成请求 |
| C | model-context 投影时根据 `turn_end(cancelled)` 生成取消提示 |

约束：

- 项目原则是“模型可见即已记录”。
- 选 C 时，提示文本也需要能在 Session 中审计。
- 建议 B 或 C，并同时做 A。

需要确认：

- 取消后是否允许模型继续旧任务。
- 取消提示是否进入 Token 统计。
- 取消提示放在下一轮 user 消息前还是后。

### 4.2 慢连接丢帧

现状：

- Host 每连接队列 64 帧。
- 满时只丢当前连接。
- 页面已有 `after_seq` 断点检查。
- 但尾部丢帧没有后续帧触发检查。

建议：

- 队列溢出时把连接标记为 `needs_resync`。
- 队列恢复后发送 `resync` 控制帧。
- 或直接关闭该连接，让 SDK 重连并 resync。
- 增加 queue depth、drops、resync 次数指标。

代码位置：

- `internal/webui/publisher.go:110`。
- `internal/webui/publisher.go:124`。
- `plugins/session/ui/main.js:1703`。
- `plugins/session/ui/main.js:2466`。

### 4.3 工具错误文案

现状：

- `file_write` 返回 `statat ...` 等底层错误。
- 用户和模型都难以判断下一步。

建议：

- `mapPathError` 输出路径、动作和稳定错误码。
- 目录路径返回 `path_is_directory`。
- 不存在的父目录返回 `path_not_found`。
- 保持现有错误码不变，只改消息。

### 4.4 W1 提前收尾

这更像模型行为，不是平台终止。

可选改进：

- 系统提示要求完成计划后再结束。
- 对“下一步/稍后/现在补充”这类收尾做启发式检测。
- 长任务换更强模型。

不建议在平台层做文本启发式。
先把 W1 作为模型质量观察项。

## 5. 契约与决策

需要先确认的点：

1. P0 是否只做 Phase 1，还是直接做 Phase 1＋2。
2. U8 采用 A、B 还是 C。
3. 取消语义采用 A、B 还是 C。
4. 慢连接溢出是主动 resync，还是直接断开重连。
5. `chatTurnInfoUI.error` 是新增字段，属于接口增量，是否接受。

## 6. 验收矩阵

| 问题 | 验收 |
|---|---|
| P0 | 30 次刷新＋W1；session 方法持续响应；页面恢复 |
| U4 | 停止 llm 后失败轮可见；错误可理解；刷新后保留 |
| U7 | 改默认模型后新建会话使用新值；手选不被覆盖 |
| U8 | 切换工作区后列表、选中、上下文一致 |
| S8 | 重启插件后页面自动显示终态 |
| 取消 | 刷新后可见“已取消/部分输出” |
| 慢连接 | 丢帧后自动 resync 或重连；尾部状态可见 |
| S4 | 推送倍率不超过 2.2× |
| S7 | 慢页不驱逐其他页；恢复后状态一致 |
