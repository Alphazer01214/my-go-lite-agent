# WebUI Browser Call 与 UI Push 交接

> 状态：已废止
> 取代者：[2026-10-19-webui-refresh-transport-plan.md](2026-10-19-webui-refresh-transport-plan.md) 与 [docs/platform/webui.md](../../platform/webui.md)
> 说明：单活动连接的 UI Push 模型已被多连接广播与 hello/resync 取代。

**日期**：2026-10-15  
**分支**：`webui`  
**状态**：Browser Call、UI State/Command 和 Diagnostic Push 已完成，准备进入管理面。  
**服务地址**：`http://127.0.0.1:2333/`

## 1. 交接结论

本次完成 WebUI 通信主干：

1. 浏览器可以调用插件 Capability。
2. 浏览器可以接收归因 Event 和最终 Response。
3. Plugin Runtime 可以推送 UI State。
4. Plugin Runtime 可以推送 UI Command。
5. WebUI 可以接收 Diagnostic Push。
6. 浏览器断开不取消后端业务 Call。
7. Host 继续使用现有 Frame 和 pending 闭环。

本次没有实现 Settings、Plugins 页面和插件生命周期管理。

## 2. 工作区状态

Git 工作区尚未提交，且同时存在：

- 已暂存改动；
- 未暂存改动；
- 未跟踪的新实现和测试。

交接时不要假设工作树干净。主要未跟踪实现：

```text
cmd/liteagent-server/server_test.go
internal/host/call.go
internal/host/call_test.go
internal/host/ui.go
internal/host/ui_test.go
internal/webui/
pluginsdk/webui_test.go
docs/archive/progress/2026-10-15-webui-shell-loader-development-log.md
```

工作区还保留切片 1、Manifest、插件文档和平台文档整理改动。

## 3. 架构 seam

### 3.1 Browser Call

```text
Browser
  → POST /bridge/call
  → internal/webui
  → Host.CallWithEvents
  → Plugin
```

`internal/webui` 保留浏览器请求 ID。Host 使用内部 `host-N` ID。输出 Event 和 Response 时，WebUI 统一重写为浏览器请求 ID。

浏览器不提交 `from`。WebUI 使用可信身份：

```text
from = "webui"
```

### 3.2 UI Push

```text
Plugin Runtime
  → pluginsdk.WebUI
  → host.ui.state / host.ui.command
  → Host.UIPublisher
  → internal/webui
  → GET /bridge/ui
  → Component.setData / runCommand
```

Host 只负责解析 Host 方法并传递可信 `from`。WebUI 负责：

- Mount 属主校验；
- State/Command Envelope；
- Diagnostic Envelope；
- 单活动连接；
- 有界 FIFO 队列；
- 浏览器离线丢弃。

## 4. 当前接口

### 4.1 Host

`E:\Projects\my-go-lite-agent\internal\host\call.go`：

```go
func (h *Host) CallWithEvents(
    from string,
    capability string,
    method string,
    payload json.RawMessage,
    onEvent func(*Frame),
) (*Frame, error)
```

`E:\Projects\my-go-lite-agent\internal\host\ui.go`：

```go
type UIPublisher interface {
    PublishState(from string, payload json.RawMessage) (json.RawMessage, error)
    PublishCommand(from string, payload json.RawMessage) (json.RawMessage, error)
}
```

Host 方法：

```text
host.ui.state
host.ui.command
```

错误码：

```text
ui_not_mounted
ui_owner_mismatch
```

### 4.2 pluginsdk

`E:\Projects\my-go-lite-agent\pluginsdk\webui.go`：

```go
func (p *Plugin) WebUI() *WebUI
func (u *WebUI) SetState(mountID string, state json.RawMessage) error
func (u *WebUI) SendCommand(mountID string, command string, payload json.RawMessage) error
```

插件不能指定其他插件的 Mount。目标地址由当前插件名和 `mount_id` 组成。

### 4.3 Browser SDK

`E:\Projects\my-go-lite-agent\internal\webui\static\lite-agent.js`：

```js
LiteAgent.call(capability, method, payload, onEvent)
LiteAgent.onUI(handler)
LiteAgent.onDiagnostic(handler)
LiteAgent.mountKey(element)
```

`call` 成功时 resolve 最终 Response 的 `payload`。失败时 reject 带 `code` 的 Error。

## 5. HTTP 契约

当前路由：

```text
GET  /
GET  /platform/*
GET  /plugins/<name>/*
GET  /bridge/bootstrap
GET  /bridge/ui
POST /bridge/call
```

`POST /bridge/call` 返回 `application/x-ndjson`：

```text
Event*
Response
```

每条 Event 和 Response 都使用浏览器请求 ID。

`GET /bridge/ui` 返回：

```text
UI State Push
UI Command Push
Diagnostic Push
```

UI 流只有一个活动连接。新连接替换旧连接。

## 6. 已实现文件

### Host

- `internal/host/call.go`
- `internal/host/ui.go`
- `internal/host/host.go`
- `internal/host/transport.go`
- `internal/host/call_test.go`
- `internal/host/ui_test.go`

### WebUI

- `internal/webui/server.go`
- `internal/webui/bridge.go`
- `internal/webui/publisher.go`
- `internal/webui/wire.go`
- `internal/webui/registry.go`
- `internal/webui/bootstrap.go`
- `internal/webui/static/index.html`
- `internal/webui/static/shell.css`
- `internal/webui/static/app.js`
- `internal/webui/static/loader.mjs`
- `internal/webui/static/lite-agent.js`
- `internal/webui/*_test.go`
- `internal/webui/*.test.mjs`

### SDK 与协议

- `pluginsdk/webui.go`
- `pluginsdk/webui_test.go`
- `protocol/errors.go`
- `cmd/liteagent-server/server.go`

## 7. 关键行为

1. `CallWithEvents` 复用 `WaitHost` 和 pending。
2. 归因 Event 按调用顺序交付。
3. 浏览器断开不取消后端 Call。
4. 断开后的 Event 和 Response 不重放。
5. `host.*` 从 Browser Call 拒绝。
6. 浏览器提交 `from` 会得到 `bad_payload`。
7. UI Push 入队成功不表示浏览器已经接收。
8. 浏览器离线或队列满时，Push 被丢弃。
9. State 和 Command 只发送到当前插件拥有的 Mount。
10. Diagnostic 不要求 Mount。
11. Diagnostic 快照保存在 WebUI 内存中。
12. 浏览器刷新后通过 Bootstrap 和插件读取方法恢复状态。

## 8. 遇到的问题与解决方法

1. **Event 回调可能阻塞 Host 读取循环。**  
   WebUI 使用有界非阻塞队列，最终 Response 保持顺序。

2. **浏览器 ID 与 Host pending ID 不同。**  
   WebUI 保留浏览器 ID，输出前统一重写。

3. **UI Publisher 为空时会提前取方法值。**  
   Host 在 `hostUIPublish` 中先读取 Publisher，再按方法分派。

4. **UI 流建立时客户端等待首个输出。**  
   `/bridge/call` 和 `/bridge/ui` 在响应头后立即 Flush。

5. **JSON 尾随数据判断使用了错误的 EOF 类型。**  
   改为使用 `io.EOF` 判断单对象结束。

6. **浏览器 `networkidle` 会等待常驻 UI 流。**  
   浏览器验证使用 `domcontentloaded`，不把长连接当作页面加载失败。

## 9. 验证记录

命令：

```text
go test ./...
go test -race ./internal/host ./internal/webui ./pluginsdk
node --test internal/webui/loader.test.mjs internal/webui/lite-agent.test.mjs
go build ./cmd/liteagent-server
```

结果：

- Go 全量测试通过；
- Host、WebUI、pluginsdk race 测试通过；
- Browser SDK 和 Loader 共 7 项测试通过；
- 构建通过；
- Playwright 实际调用 `session.list` 成功；
- 断开请求不取消后端 Call；
- UI Stream 收到 State、Command 和 Diagnostic；
- `git diff --check` 通过。

已知问题：

```text
go vet ./...
plugins/context-manager/main.go:33:
struct field messages has json tag but is not exported
```

该问题早于本次 WebUI 工作，当前未修复。

## 10. 非目标

本阶段未实现：

- Settings Renderer；
- Plugins 页面；
- 插件启停；
- 插件恢复；
- 禁用名单持久化；
- 路由回收；
- Call 超时；
- Event Replay；
- 多浏览器同步；
- 插件 JavaScript 沙箱。

## 11. 下一步

进入管理面：

1. 扩展插件状态快照；
2. 实现 Plugins 页面；
3. 实现 Settings Renderer；
4. 实现插件启停和恢复；
5. 增加慢客户端、重连和生命周期集成测试。

继续使用现有 Host、WebUI 和 pluginsdk seam，不创建第二套路由或 pending 机制。
