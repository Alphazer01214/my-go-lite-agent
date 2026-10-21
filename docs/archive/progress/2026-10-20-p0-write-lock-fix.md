# P0 写锁与超时修复

> 状态：当前实现
> 说明：写锁拆分与本地超时已实现在 `pluginsdk/plugin.go`。

## 1. 背景

2026-10-20 全量测试发现快速刷新会卡死 session 插件。
现场表现为 `session.*` 全部超时。
页面变空，Turn 停住。
只有重启插件或 Host 才能恢复。

测试记录见 [2026-10-20-full-test-report.md](2026-10-20-full-test-report.md)。
方案研究见 [2026-10-20-full-test-fix-plan.md](2026-10-20-full-test-fix-plan.md)。

本次修复只处理 P0 死锁。
流式发布合并与 UI 状态裁剪不在本次范围。

## 2. 根因

问题来自插件 SDK 与 Host 的同步写回。

1. handler 调用 `ui.SendCommand`。
2. SDK 写 stdout 时持有 `p.mu`。
3. Host 读循环把响应同步写回同一插件。
4. 插件 `Listen` 处理响应时也需要 `p.mu`。
5. 写 stdout 因管道满而阻塞。
6. `Listen` 无法取得 `p.mu`，无法读取 stdin。
7. Host 写回也阻塞。
8. 双方形成循环等待。

关键代码：

- `pluginsdk/plugin.go:655` 的 `writeFrame` 原实现。
- `pluginsdk/plugin.go:450` 的 `waitResult` 原实现。
- `internal/host/transport.go:328` 的同步写回。
- `internal/host/transport.go:499` 的 Host 写 stdin。

原超时机制也依赖同一条写路径。
超时 goroutine 调用 `writeCancel`。
`writeCancel` 同样需要 `p.mu`。
所以超时本身也会被死锁卡住。

## 3. 实现

### 3.1 拆分写锁

`Plugin` 新增 `writeMu`。

`mu` 只保护状态：

- `handlers`
- `pending`
- `inbound`
- `inboundRecent`

`writeMu` 只保护 stdout 写：

- `writeFrame`
- `EmitInContext`
- `EmitWithIDInContext`
- `writeCancel`

结果：

- `Listen` 的 `complete` 不再等待 stdout 写。
- 插件可以持续消费 stdin。
- Host 的同步写回可以完成。
- 循环等待被切断。

### 3.2 超时本地生效

`waitResult` 直接监听 `ctx.Done()`。

超时或取消时：

1. 标记 outbound call 已取消。
2. 异步尝试发送 cancel 帧。
3. 从 `pending` 删除该 call。
4. 立即返回 `call_deadline_exceeded` 或 `call_cancelled`。

不再等待原 Request 的终态。
迟到的 Response 因 pending 已删除而被丢弃。

新增 `removePending`。
它只在 id 仍指向同一个 call 时删除。
避免迟到响应误删新 call。

### 3.3 Host 侧

本次不修改 Host 的 `plg.mu`。
该锁当前只用于串行写 stdin。
拆分锁不是解除死锁的必要条件。

Host 读循环仍是同步写回。
Phase 2 可以改为每插件 writer goroutine。
本次先用 SDK 修复验证 P0。

## 4. 锁顺序

修复后的规则：

- 状态锁与写锁不能嵌套持有。
- 写 stdout 时只持有 `writeMu`。
- 完成 pending 时只持有 `mu`。
- `waitResult` 超时路径不等待任何写操作。
- cancel 帧是尽力而为，不参与超时判定。

新增测试覆盖这些规则。

## 5. 测试

新增文件 `pluginsdk/write_lock_test.go`。

### 5.1 超时不依赖 cancel 写

`TestCallDeadlineDoesNotDependOnCancelWrite`：

- 发起一个无人响应的 Call。
- Context 在 50 ms 后超时。
- 断言返回 `call_deadline_exceeded`。
- 断言 pending 已清空。

### 5.2 阻塞写不阻塞完成

`TestBlockedStdoutDoesNotBlockComplete`：

- 用 4 MiB Frame 填满 stdout 管道。
- 确认 `writeFrame` 已阻塞并持有 `writeMu`。
- 此时调用 `complete`。
- 断言 `complete` 在 1 秒内完成。

该测试直接覆盖原死锁条件。

全量检查：

```bash
go vet ./...
go test ./... -count=1
go test -race ./pluginsdk ./plugins/session -count=1
node --test internal/webui/*.test.mjs
```

结果全部通过。

## 6. 实测

环境与全量测试相同。
真实模型 `mimo-v2.6-flash`。

### 6.1 流式 Turn＋30 次刷新

步骤：

1. 新建会话 `s-1792490676016595500`。
2. 发送约 1500 字长文任务。
3. Turn 进入 streaming。
4. 用无头 Chrome 执行 30 次刷新，间隔 200 ms。
5. 等待 Turn 完成。

结果：

| 指标 | 结果 |
|---|---|
| 最后页面恢复 | 1 ms 内就绪 |
| `session.state` 调用 | 79 次 |
| `session.state` 最大响应 | 191,364 字节 |
| `session.state` 最大耗时 | 36 ms |
| `session.workspace.list` 探测 | 200，持续可用 |
| Turn 终态 | `completed` |
| `queue full` | 0 |

刷新窗口约 6.7 秒。
刷新后页面仍显示生成的 Turn。
停止按钮可用。
没有出现空页面或能力超时。

### 6.2 大 State＋多次刷新

步骤：

1. 选择 D2 会话 `s-1792482626506121700`。
2. 该会话有 250 条消息与约 1.2 MiB 消息。
3. 执行 10 次刷新，间隔 200 ms。
4. 每次刷新后读取 `session.state`。

结果：

| 指标 | 结果 |
|---|---|
| `session.select` | 156 ms |
| `session.state` 首次 | 179 ms，3.98 MB |
| 10 次刷新中的 `session.state` | 3 次，最大 5.30 MB |
| 最大 `session.state` 耗时 | 249 ms |
| `session.workspace.list` 探测 | 200，1.5 ms |
| 页面 | 200 条记录，选中正确 |

大 State 不再触发卡死。
状态体积仍然超过性能预算。
这部分留到后续优化。

## 7. 剩余风险

### 7.1 cancel 帧可能未送达

stdout 阻塞时，异步 cancel 可能仍在等待 `writeMu`。
调用方已经返回，不会再次卡死。
Host 侧可能暂时保留原 Call。
Phase 2 应把它与本 Call 的超时治理一起处理。

### 7.2 流式发布仍逐帧发送

`stream.update` 仍然每一个 delta 发一次 `host.ui.command`。
本次没有做合并。
高负载下会增加往返次数与 UI 队列压力。
Phase 2 的 latest-value outbox 仍然建议实施。

### 7.3 Host 仍同步写回

Host 读循环写回调用方时可能阻塞。
插件持续读 stdin 后可以恢复。
如果第三方插件不读 stdin，仍可能拖住自己的读循环。
Phase 2 可改为每插件 writer goroutine。

### 7.4 大 State 预算未达标

D2 的 State 达到 5.30 MB。
高于每 Mount 2.2 MiB 与总 State 2.5 MiB 的目标。
这不是 P0 死锁，但仍是性能缺口。

## 8. 回滚

本次改动只涉及 `pluginsdk/plugin.go` 的锁与等待逻辑。
没有修改线协议。
没有修改 Manifest。
没有修改 Session Fact 格式。
没有修改 UI State 字段。

回滚方式：

- 还原 `pluginsdk/plugin.go`。
- 删除 `pluginsdk/write_lock_test.go`。
- 重新构建 dist。

## 9. 后续

建议下一步做 Phase 2：

- session stream latest-value outbox。
- Host 每插件 writer goroutine。
- UI 队列溢出后的显式 resync。
- 大 State 窗口与预算优化。

Phase 3 的 U4、U7、U8、S8 修复另立任务。
