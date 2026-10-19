# WebUI 刷新问题改造计划

日期：2026-10-19
状态：方案已确认，P0、P1、P2 已实施
范围：WebUI 传输层、Browser Call 生命周期、Agent Turn 终态、前端重连

## 1. 背景与定位

浏览器刷新会触发两类问题。第一，进行中的对话被中止，Session 停在 `running`。第二，高速刷新后插件内容不再更新。两个问题分别位于 Browser Call 生命周期和 UI 推送连接模型。

### 1.1 刷新中止 Turn

当前链路如下。

1. `internal/webui/bridge.go` 把浏览器请求的 `r.Context()` 传给后端 `CallWithEvents`。
2. 刷新时浏览器中止 `POST /bridge/call`，Host 收到 ctx 取消后向 Agent 转发 cancel。
3. Agent 取消 `llm.complete`，上游模型流被断开。
4. `runTurn` 的错误收尾使用已取消的 `st.ctx` 写 `turn_end`，写入失败被忽略。
5. Session 永久停在 `turn.status=running` 和 `stream.status=streaming`，`loop.cancel` 返回 `cancelled:false`，页面无法恢复。

实测证据：假 LLM 只发出 1/100 个 chunk 就被 abort；服务端 trace 出现 `loop.turn` 和 `llm.complete` 的 cancel 帧；刷新后 Session 的 Fact 停在 `request_header`，没有 `turn_end`。

### 1.2 高速刷新后 UI 推送中断

当前链路如下。

1. `internal/webui/publisher.go` 只保存一个 UI 连接。
2. 新连接注册时立即驱逐旧连接，注册前不校验请求是否存活。
3. 高速刷新会产生大量被浏览器中止的 `/bridge/ui` 请求。
4. 某个中止请求的 handler 如果晚于最后一页注册，会驱逐最后一页，然后清空连接槽。
5. 最后一页收到 `evicted`，前端停止重连，后续插件推送全部丢失。

实测证据：同包确定性测试出现“将死请求驱逐活动连接”红灯；传输层压测 40 轮全部 evicted，推送 0 条；页面级压测中 `session.append` 的新消息不再进入 DOM。

### 1.3 文档冲突

文档对浏览器断开的语义不一致。`docs/webui.md` 一处写断开取消 Browser Call，另一处写浏览器关闭不影响后端 Call。`docs/development.md` 也同时存在两种表述。当前实现和 `internal/webui/disconnect_test.go` 选择取消语义，与产品目标不符。

## 2. 共识方案

四个决策已经确认。

- 通信方案采用 A：保留 `POST /bridge/call` 和 `GET /bridge/ui`，UI 改为多连接广播，Browser Call 生命周期与 HTTP 请求解耦。
- UI 重连采用 resync 重建，不做事件游标重放。
- Turn 接口本轮保持同步 `loop.turn`，只解耦后端执行与浏览器连接。
- State、Command、Diagnostic 全部广播给所有页面。

目标语义如下。

1. 浏览器是可重启的观察端，业务执行和状态真源都在后端。
2. 浏览器断开只停止写回，不取消后端 Call。
3. 任意时刻允许多个页面同时接收 UI 推送。
4. 重连通过 Bootstrap 和插件读取方法重建状态，不重放事件。
5. Turn 的任何终态都必须写入 `turn_end`，不允许留下 `running` 残留。

## 3. 待修改条目

### 3.1 UI 推送与连接

| 编号 | 当前实现 | 期望结果 |
|---|---|---|
| A1 | `Server.connection` 只保存一个连接；新连接通过 `replaceUIConnection` 立即驱逐旧连接 | `Server` 保存连接集合；新增和移除互不影响；State、Command、Diagnostic 广播给所有活动连接；删除 `evicted` |
| A2 | `enqueueUI` 只向唯一连接入队；队列满或断开就返回 `false` | 每个连接独立有界队列；慢页面只丢自己的帧；至少一个页面收到就向插件返回成功；连接退出时从集合移除 |
| A3 | 断线后固定 1 秒重连；连接成功没有握手；页面不知道发生过断线 | UI 流建立后发送 `hello`；SDK 提供连接状态和 `onResync`；断线后指数退避重连；重连后重新拉取 Bootstrap 和插件状态 |
| A4 | 只有最新连接收到 Command，多标签页行为没有定义 | Command 与 State 一样广播；Component 契约要求命令对所有可见标签页安全 |
| A5 | 无连接或队列满时，Session 静默忽略发布错误 | Host 记录连接建立、断开和队列丢弃；插件发布是否成功按“至少一个页面收到”判定 |

### 3.2 Browser Call 与 Turn

| 编号 | 当前实现 | 期望结果 |
|---|---|---|
| B1 | `serveBrowserCall` 把 `r.Context()` 传给后端，浏览器断开就取消整个调用 | 后端使用不随浏览器断连取消的上下文；断开只关闭写循环；未写出的 Event 和 Response 丢弃；Call 继续等待插件终态 |
| B2 | 刷新、关页、网络断开都会隐式取消正在执行的 Turn | 刷新和断网不取消；用户点“停止”走 `loop.cancel`；Host 关闭时统一取消 |
| B3 | 错误收尾用已取消的 `st.ctx` 写 `turn_end`，写入失败被忽略 | 所有终态使用独立收尾上下文（`WithoutCancel` 加短超时）；completed、cancelled、error、max_steps 都写 `turn_end`；入站取消按取消路径处理 |
| B4 | 前端 await 整个 `loop.turn`，但结果只用于 busy 和错误提示 | 保留同步 `loop.turn`，但它的返回值只作为当前页面的附加信号；刷新后以 Session Fact 和 UI 推送为准 |
| B5 | Session 只在 Turn 所属插件不可用时补写 `turn_end`；服务端重启后旧 Turn 不会恢复 | 本轮先保证正常取消路径不产生残留；服务端重启和插件崩溃后的 Turn 对账单独排期 |

### 3.3 前端与插件契约

| 编号 | 当前实现 | 期望结果 |
|---|---|---|
| C1 | Component 契约只有 `setData`、`runCommand`、`connectedCallback`、`disconnectedCallback` | 增加可选 `resync()`；Shell 在收到 resync 后调用已挂载组件的 `resync()`；组件未实现时回退为重新挂载 |
| C2 | SDK 收到 `evicted` 后永久停止重连；Shell 显示“已由其他标签页接管” | 删除 `onEvicted` 和接管提示；增加 `onStatus`、`onResync`；顶栏显示“已连接”或“重连中”；重连后自动重建状态 |
| C3 | 插件通过 `SetState` 和 `SendCommand` 发布，Host 校验 Mount 属主 | 保持插件接口不变；只把 Host 内部从单队列改成广播；插件不需要感知页面数量 |

### 3.4 文档与测试

| 编号 | 当前实现 | 期望结果 |
|---|---|---|
| D1 | 文档同时存在“断开取消”和“关闭不影响业务”两种表述；Evicted 契约仍保留 | 统一为“断开不取消后端、UI 多连接广播、重连 resync、不重放事件”；删除 Evicted 契约 |
| D2 | `disconnect_test.go` 断言断开取消后端；没有广播和 resync 测试 | 反转断言语义；补充广播、无驱逐、重连恢复、Turn 终态和浏览器 E2E 回归 |

## 4. 实施阶段

三个阶段的改动逐层叠加，每个阶段完成后跑对应测试。

| 阶段 | 内容 | 验收 |
|---|---|---|
| P0 | B1、B2、B3、B4；反转 `disconnect_test.go` | Turn 中刷新后模型流继续到结束；不再出现 `call_cancelled`；Session 不残留 `running`；显式取消仍正常 |
| P1 | A1、A2、A4、A5；删除前端 Evicted 逻辑和接管提示 | 多标签页同时收到推送；高速刷新后最后一页保持连接；`session.append` 能实时更新 DOM |
| P2 | A3、C1、C2；实现 `hello` 和 resync | 断流或服务端重启后重连，Bootstrap、Diagnostics、当前会话和流式草稿恢复；重连期间新增的消息可见 |
| P3 | D1、D2；更新文档和 E2E 脚本 | `go test ./...` 全绿；浏览器刷新与 Turn 脚本回归通过 |

## 5. 范围与非目标

本轮做以下内容。

- WebUI 连接层、Browser Call 生命周期、Agent Turn 终态。
- 前端 SDK、Shell、Session UI 的 resync。
- 文档和回归测试。

本轮不做以下内容。

- WebSocket 传输。
- 事件游标重放。
- `start/observe` 异步 Turn 模型。
- 多用户、远程访问、插件沙箱。
- 服务端重启后的 Turn 对账。
- 插件业务方法调整。

## 6. 风险与取舍

- Command 广播要求组件对多标签页安全。焦点类命令需要检查 `document.visibilityState`。
- `loop.turn` 仍是长 HTTP 请求。页面刷新后响应会丢失，但后端继续执行，状态由 Session 重建。
- resync 会产生一次状态读取突发。频率低，v1 本机单用户场景可以接受。
- 多连接广播改变 v1“单浏览器单连接”假设。多标签页从互抢变成同时接收，需要同步更新文档。
- 不做事件重放，重连到 resync 完成之间可能有短暂陈旧窗口。窗口由组件读取状态填补。
- 发布成功语义改为“至少一个页面收到”。没有页面时仍返回失败，由插件决定是否重试。

## 7. 验收与回归

单元测试覆盖以下场景。

- 浏览器断开后后端 Call 继续完成，未写出的帧被丢弃。
- 两个 UI 流同时收到 State 和 Diagnostic，第三个连接不驱逐前两个。
- 断线重连触发 resync，Session 状态恢复。
- 取消、错误和工具途中取消都写 `turn_end`，不再留下 `running`。

浏览器 E2E 覆盖以下场景。

- 高速刷新 50 次，最后一页保持连接，追加消息可见。
- Turn 中刷新，模型流继续，刷新后草稿继续增长，最终正文落地。
- 双标签页同时接收推送，旧页不再冻结。
- 手动断流后重连，状态和草稿恢复。

回归入口如下。

```text
go test ./... -count=1
```

浏览器脚本曾在 `.tmp-e2e/repro/` 中运行，验证完成后已随临时环境清理。

## 8. 后续待定

以下内容不属于本轮，但会影响长期稳定性。

- 服务端重启或插件崩溃后，如何对账未闭合的 Turn。
- 是否把 `loop.turn` 拆成 start 和 observe，彻底移除长 HTTP 请求。
- 是否需要带游标的增量重放，减少 resync 的读取突发。
- 是否需要统一页面级通道（方案 B）或 WebSocket（方案 C）。

## 9. P0 实施记录

日期：2026-10-19

P0 已完成以下改动。

- `internal/webui/bridge.go`：Browser Call 使用脱离 HTTP 请求的后端上下文；断开只停止写回。
- `plugins/agent/turn.go`：终态写入使用独立收尾上下文；入站取消走 cancelled 路径；所有终态写 `turn_end`。
- `internal/webui/disconnect_test.go`：断言改为“断开后后端继续运行”。
- `plugins/agent/turn_test.go`：新增入站取消回归，fake SDK 按 ctx 取消。
- `docs/webui.md`、`docs/development.md`：同步 Browser Call 断开语义。

验证结果如下。

- `go test ./... -count=1` 全绿。
- `go test -race ./internal/webui ./plugins/agent -count=1` 全绿。
- E2E 短等待：Turn 中刷新后草稿从 `[0]` 增长到 `[38]`，页面保持“生成中”。
- E2E 长等待：假 LLM 完成 100/100 个 chunk，trace 无 cancel 帧；Session 写入 `turn_end`，状态为 completed。

P1 已完成。P2 见下节。

## 10. P1 实施记录

日期：2026-10-19

P1 已完成以下改动。

- `internal/webui/server.go`、`internal/webui/publisher.go`：连接改为集合；每个连接独立有界队列；State、Command、Diagnostic 广播；删除 Evicted 和驱逐逻辑。
- `internal/webui/publisher.go`：连接建立、关闭和队列首次丢弃写入日志；服务端接入 `SetLogFunc`。
- `internal/webui/static/lite-agent.js`、`app.js`、`shell.css`：删除 `onEvicted`、停止重连逻辑和“已由其他标签页接管”提示。
- `internal/webui/publisher_test.go`、`ui_stream_test.go`：新增广播、队列隔离和双 HTTP 流测试。
- `docs/webui.md`、`ARCHITECTURE.md`：改为多连接广播语义，删除 Evicted 契约。

验证结果如下。

- `go test ./... -count=1` 全绿。
- `go test -race ./internal/webui ./plugins/agent -count=1` 全绿。
- 中止连接压测：40 次中止 `/bridge/ui` 请求后，当前页保持 `ready`，追加消息正常进入 DOM。
- 快速刷新：50 次刷新后页面保持连接，`session.append` 的标记消息可见。
- 双标签页：两个页面同时收到 State 推送；`stream.update` 的 Command 在两个页面都更新草稿。

P2 的重连 resync 已完成，见下节。

## 11. P2 实施记录

日期：2026-10-19

P2 已完成以下改动。

- `internal/webui/publisher.go`：每条 UI 连接建立后先发送 hello。
- `internal/webui/static/lite-agent.js`：新增 `onStatus`、`onResync`；hello 触发重连检测；断线后指数退避重连。
- `internal/webui/static/app.js`：重连后重新拉取 Bootstrap；服务端诊断快照整体替换；顶栏显示连接、重连和错误状态。
- `internal/webui/static/loader.mjs`：新增组件 `resync()`；组件未实现时重挂；resync 失败按 0、250、500、1000、2000 ms 重试。修复 `wanted` 集合把 spec 误传给 `mountKeyFor`、导致每次 load 都重挂全部组件的问题。
- `plugins/session/ui/main.js`：History、Chat、Trace 三个组件实现 `resync()`；失败向上抛出，由 loader 重试。
- `plugins/session/store.go`：`uiRevision` 以进程启动微秒时间为起点，保证跨重启递增。
- `internal/webui/*.test.mjs`、`internal/webui/ui_stream_test.go`、`plugins/session/ui_state_test.go`：补 hello、status、resync、重试、重载保留和跨重启 revision 测试。
- `docs/webui.md`、`plugins/session/communication.md`：补 hello、`onStatus`、`onResync`、组件 `resync()` 和 revision 跨重启契约。

验证结果如下。

- `go test ./... -count=1` 全绿。
- `go test -race ./internal/webui ./plugins/agent ./plugins/session -count=1` 全绿。
- 前端 JS 测试 13 项全绿。
- 停服重连：停服期间直接向 Session 日志追加 Fact；页面显示“重连中”。重启后页面恢复“已连接”，新增 Fact 通过 resync 出现在对话中，`recovered=true`。
- P0 回归：Turn 中刷新后草稿继续增长。
- P1 回归：中止 UI 请求不驱逐页面；双标签页同时收到 State 和 Command。

以下项目仍不在本轮范围。

- 服务端重启时正在执行的 Turn 对账（B5）。
- 事件游标重放。
- `loop.turn` 的 start/observe 异步模型。
