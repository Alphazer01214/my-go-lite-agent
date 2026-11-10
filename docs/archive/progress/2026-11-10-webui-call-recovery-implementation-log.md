状态：当前实现

# WebUI 调用恢复实现日志

## 1. 背景

实测发现两个交互问题：

1. 回合运行中刷新页面，运行标记、停止按钮和在途文本全部丢失；
2. 工具调用与工具结果已经落库，但回合结束前聊天区看不到。

根因在 Host 的 WebUI 传输层：逻辑调用没有身份，事件通道与 HTTP 响应同生命周期。
刷新后后端继续运行，但浏览器没有任何入口接回原调用。

## 2. 约束

- 插件间与插件到 Host 的协议不变：capability、method、payload、事件类型都不改。
- 不引入 WebSocket 依赖，继续使用 NDJSON 流。
- 只使用现有业务方法：`loop.turn`、`loop.cancel`、`session.info`、`session.messages`、`session.facts`。
- 插件 Manifest、Mount Registry、`ui.state` / `ui.command` 语义不变。

## 3. 方案

Host 从"请求即流"改为"调用登记 + 可 attach 流"：

```text
浏览器
  ├─ POST /bridge/call                 原同步调用，行为不变
  ├─ GET  /bridge/calls                可 attach 的调用快照
  ├─ GET  /bridge/calls/{id}?after=    单调用重放 + 实时 + 终态
  └─ GET  /bridge/ui?after=            UI 总线；call 生命周期与诊断可重放

internal/webui
  ├─ callHub     call ID → 请求、事件环、状态、结果、订阅者、TTL
  └─ replay ring 只重放 call 与 diagnostic
```

1. `serveBrowserCall` 在调用开始登记 `callRecord`。
   每个事件先入有界缓冲，再写原响应。
2. 首个事件时在 `/bridge/ui` 广播 `phase=started`；终态时广播 `phase=finished`。
3. 有事件的调用结束后保留 10 分钟；无事件的短调用立即删除。
4. 页面刷新后调用 `/bridge/calls`，对运行中的 `loop.turn` 打开 `/bridge/calls/{id}`。
   attach 流按 `replay → event → replay_end → 实时 event → result` 输出。
5. 会话 UI 用重放事件恢复本地 turn、运行标记和 In-flight 文本。
   停止继续调用插件现有的 `loop.cancel`。
   attach 断线重试时带上最后处理的 per-call seq，服务端只补发后续事件。
6. 恢复期间每 2 秒轮询 `session.info`；消息数增加时重新加载 `session.messages`。
   Turn 结束后由 `call` 生命周期或 attach 的 `result` 触发消息刷新。

## 4. 实现切片

| 切片 | 文件 | 内容 |
|---|---|---|
| 调用登记 | `internal/webui/calls.go` | 事件环、订阅、attach 快照、TTL、生命周期回调 |
| 调用接入 | `internal/webui/bridge.go` | 事件先入 Registry；终态写入结果；浏览器断开不取消 |
| 新端点 | `internal/webui/calls_http.go` | `/bridge/calls` 与 attach 流的编解码 |
| UI 总线 | `internal/webui/publisher.go` | `seq` / `rseq`、`call` / `ping` / `resync`、重放环、心跳 |
| 路由与字段 | `internal/webui/server.go` | 新路由与 Call Registry 初始化 |
| 浏览器 SDK | `internal/webui/static/lite-agent.js` | `listCalls`、`watchCall`、`onCall`、游标去重 |
| 会话恢复 | `plugins/session/ui/store.js` | 恢复 turn、绑定新会话、停止、结束刷新、消息轮询 |
| 展示细节 | `plugins/session/ui/chat.js` | 缓冲截断提示 |

## 5. 关键决策

**为什么插件零改动**

Host 已经能看到浏览器调用产生的全部事件和最终响应。
把这些帧按调用缓存并在新连接上重放，比要求每个插件实现 `status` 和流基线更通用，也不破坏协议。

**为什么只保留有事件的调用**

`session.info`、`files.stat` 这类一元调用没有可恢复内容。
结束时立即删除，避免 `/bridge/calls` 被高频短调用淹没。

**为什么重放环只放 `call` 和 `diagnostic`**

`ui.state` 是插件定义的快照推送。
回放旧快照会让面板短暂回退；它的恢复路径是 `hello` / `resync` 后的 Component `resync()`。

**为什么工具过程仍用轮询**

插件协议不变，Host 不解析 `session.append`。
`session.info` 的 `message_count` 变化触发一次 `session.messages` 重载，代价可控，并且不改插件。
后续若插件增加事实事件，可以把这层轮询替换为推送。

**为什么没上 WebSocket**

当前缺口是逻辑流身份、缓冲和游标，不是双向传输。
NDJSON 总线加 attach 流已经覆盖聊天场景，也避免新增依赖。

## 6. 验证

单元测试（`go test ./internal/webui/... -race`）：

- 事件缓冲、attach 重放、实时订阅、终态事件；
- 无事件调用立即移除；
- 缓冲截断与 `truncated`；
- 生命周期回调只触发一次；
- 游标缺口检测与 `resync`；
- `/bridge/call` 到 attach 的完整帧顺序；
- 浏览器断开后调用仍完成并可 attach；
- `/bridge/ui?after=` 的 hello 与重放顺序。

真实环境（dist 构建，`mimo-v2.6-flash`，python `sleep 25` / `sleep 30`）：

1. 运行中刷新：恢复"生成中"、停止按钮、已用时间和历史运行点；
   网络面板出现 `/bridge/calls` 与 `/bridge/calls/{id}`。
2. 运行中可见已落库的工具调用消息；draft 与持久化消息不重复。
3. 回合结束页面自动切换为完整消息，无需再次刷新。
4. 刷新后停止：出现"本轮已取消"，运行标记消失，发送恢复。
5. `go build ./...` 与 session / subagent 测试通过。

验证中发现并修正一个缺陷：`bridge.go` 最初直接调用 `record.finish`，绕过了 Call Registry 的清理与 finish 回调。
结果是所有一元调用都留在 `/bridge/calls`，`phase=finished` 也从不广播。
改为 `callHub.finish` 后，无事件调用立即移除，终态生命周期正常广播。
补了 `TestServeBrowserCallDropsEventlessCall` 覆盖这条路径。

## 7. 已知边界与后续

- 写入仍在原 `/bridge/call` 响应流上进行，长回合仍占一条 HTTP 连接。
  下一步可增加 `background` 模式，让发起页也只消费 attach 流。
- 工具调用仍以文本 `[tool calls] N` 展示，尚未渲染结构化工具卡片。
- 只登记浏览器发起的调用；子代理 `loop.turn` 的运行态尚未出现在会话树。
  如需，可在 Host 路由层增加通用调用观察，不改插件协议。
- attach 缓冲 1 MiB；超长回合被截断时显示"部分输出未恢复"。
- Host 重启后 Call Registry 清空，恢复出的本地 turn 会被同步收尾。

## 8. 涉及文件

- `internal/webui/calls.go`
- `internal/webui/calls_http.go`
- `internal/webui/calls_test.go`
- `internal/webui/bridge.go`
- `internal/webui/publisher.go`
- `internal/webui/server.go`
- `internal/webui/static/lite-agent.js`
- `plugins/session/ui/store.js`
- `plugins/session/ui/chat.js`
- `docs/platform/internal_webui.md`
- `docs/platform/webui.md`
- `plugins/session/README.md`
