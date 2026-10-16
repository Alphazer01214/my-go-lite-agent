# WebUI Shell 与 Loader 开发日志

**日期**：2026-10-15  
**分支**：`webui`  
**状态**：切片 2 已完成，准备进入 Browser Call。  
**范围**：Host Shell、Slot、Bootstrap、Component Loader 和插件 UI 静态资源服务。

## 1. 本次结论

本次完成 WebUI 切片 2。实现边界如下。

1. `cmd/liteagent-server` 拥有 HTTP 生命周期。
2. `internal/webui` 返回 `http.Handler`。
3. Shell 提供五个 Slot。
4. Loader 按 entry 去重加载 Component。
5. 单个 Mount 失败不影响其他 Mount。
6. 切片 2 只读取 Bootstrap Diagnostic 快照。
7. Diagnostic 运行时 Push 延后到切片 4。
8. WebUI 默认监听 `127.0.0.1:2333`。

本次没有实现 Browser Call、UI Push、Settings 和 Session 前端。

## 2. 当前工作区

Git 尚未提交。工作区同时存在已暂存、未暂存和未跟踪改动。

本次切片主要新增：

- `cmd/liteagent-server/server_test.go`；
- `internal/webui/` 全部实现和测试文件。

工作区还包含此前切片 1 和文档整理改动：

- `internal/plugin/` 的 Manifest、Discovery 和 Diagnostic；
- `internal/host/constant.go`、`protocol/errors.go`；
- 根目录架构、约束和上下文文档；
- `docs/` 平台文档和 WebUI handoff；
- 各插件 README 和 communication 文档；
- `plugins/context-manager/`、`plugins/filetools/` 文档。

因此，当前差异不等于一次独立提交。提交前应按变更主题整理暂存区。

## 3. 设计决策

### 3.1 HTTP 启动 seam

`cmd/liteagent-server` 负责：

- 读取监听地址；
- 创建 `net.Listener`；
- 创建 `http.Server`；
- 处理退出信号；
- 执行 HTTP Shutdown；
- 启动和关闭 Host。

`internal/webui` 只负责：

- 创建 HTTP Handler；
- 路由 Shell、Bootstrap 和静态资源；
- 不管理进程和监听端口。

这样保持 Host 薄内核，也便于使用 `httptest` 测试 WebUI。

### 3.2 Diagnostic 边界

切片 2 展示 Bootstrap 返回的 Diagnostic 快照。

Loader 运行失败时，浏览器生成本地 Diagnostic。该记录只用于当前 Shell 展示。

以下能力留到切片 4：

- `/bridge/ui`；
- Diagnostic 运行时 Push；
- 断线重连；
- 有界队列；
- 多浏览器连接策略。

### 3.3 Loader 失败策略

Loader 执行这些规则：

- 相同 entry 只执行一次 `import()`；
- 相同 MountKey 只创建一个实例；
- entry 导入失败时跳过当前 Mount；
- `customElements.whenDefined` 超时时跳过当前 Mount；
- 失败写入本地 Diagnostic；
- 其他 Mount 继续加载；
- 失败 Mount 不自动重试；
- 刷新页面后重新加载。

## 4. 代码修改

### 4.1 HTTP Server

`cmd/liteagent-server/server.go` 新增：

- 默认地址 `127.0.0.1:2333`；
- 环境变量 `LITEAGENT_HTTP_ADDR`；
- `net.Listener` 和 `http.Server` 生命周期；
- `ReadHeaderTimeout` 和 `IdleTimeout`；
- 实际 URL 日志；
- 5 秒 HTTP Shutdown；
- Host 和 WebUI 的组合启动。

`cmd/liteagent-server/server_test.go` 覆盖默认地址、环境变量覆盖和 URL 格式。

### 4.2 WebUI Handler

`internal/webui/server.go` 新增：

- `NewHandler(registry *Registry) http.Handler`；
- embedded 静态资源；
- Plugin UI 资源解析；
- Bootstrap JSON；
- MIME 类型；
- CSP、`nosniff` 和 Referrer Policy；
- parent path 拒绝；
- GET 方法限制。

当前路由：

```text
GET /
GET /platform/*
GET /plugins/<name>/*
GET /bridge/bootstrap
```

`POST /bridge/call` 和 `GET /bridge/ui` 尚未加入。

### 4.3 Shell

`internal/webui/static/index.html` 定义：

- Host top 区；
- left、center、right、bottom Slot；
- Diagnostic 开关；
- 非模态 Diagnostic 面板。

`internal/webui/static/shell.css` 提供：

- 稳定的五区布局；
- 桌面三列布局；
- 平板双列布局；
- 手机纵向布局；
- 无横向溢出；
- 无元素重叠；
- 可读的空 Slot 状态。

`internal/webui/static/app.js` 负责：

- 读取 Bootstrap；
- 渲染 Diagnostic；
- 收集 Slot；
- 创建 Mount Loader；
- 记录 Loader 本地失败。

### 4.4 Loader

`internal/webui/static/loader.mjs` 提供 `createMountLoader`。

Loader 隐藏以下复杂度：

- entry Promise 去重；
- Custom Element 定义等待；
- 定义超时；
- MountKey 去重；
- Slot 定位；
- `data-mount-key` 写入；
- 单 Mount 错误隔离。

`internal/webui/static/lite-agent.js` 当前只提供 `mountKey`。Browser Call 相关方法留到切片 3。

### 4.5 Registry 修正

`internal/webui/registry.go` 保证 Bootstrap 的 `diagnostics` 始终为 JSON 数组。

空诊断返回 `[]`，不再返回 `null`。该行为与 `docs/webui.md` 的 Bootstrap 契约一致。

## 5. 测试

### 5.1 Go 测试

`go test ./...` 通过。

`internal/webui` 覆盖：

- Shell 和静态资源；
- MIME 类型；
- Bootstrap；
- Plugin UI 资源；
- 越界路径；
- 非法方法；
- 本地路径不泄漏；
- Manifest、Registry 和诊断快照。

`cmd/liteagent-server` 覆盖：

- 默认监听地址；
- 环境变量覆盖；
- IPv4、IPv6 URL。

### 5.2 Loader 测试

命令：

```text
node --test internal/webui/loader.test.mjs
```

结果：4 项通过。

覆盖场景：

1. entry 去重；
2. MountKey 去重；
3. entry 导入失败隔离；
4. Custom Element 定义超时隔离。

`internal/webui/loader_js_test.go` 会在 `go test` 中调用该测试。系统没有 Node 时跳过。

### 5.3 HTTP 冒烟

已实际启动 `dist/liteagent-server.exe`。

验证结果：

- `/` 返回 `200 text/html`；
- `/platform/app.js` 返回 JavaScript MIME；
- `/platform/loader.mjs` 返回 JavaScript MIME；
- `/bridge/bootstrap` 返回 JSON；
- `/plugins/session/plugin.json` 返回 `404`；
- 默认 URL 为 `http://127.0.0.1:2333/`。

### 5.4 浏览器验证

使用本机 Chrome 和 Playwright 验证：

- 桌面 `1440x900`；
- 手机 `390x844`；
- 五 Slot 无横向溢出；
- 五 Slot 无重叠；
- 手机纵向布局无空白断层；
- Diagnostic 面板非模态；
- 相同诊断自动合并；
- entry 失败会显示本地 Diagnostic；
- Mock Custom Element 成功挂载到 center Slot；
- `data-mount-key` 为 `demo:main`；
- 页面无 JavaScript 异常。

## 6. 文档同步

本次更新：

- `ARCHITECTURE.md`；
- `docs/development.md`；
- `docs/webui.md`；
- `docs/progress/2026-10-14-webui-implementation-handoff.md`。

同步内容：

- `internal/webui` 模块职责；
- HTTP 生命周期归属；
- 默认端口 2333；
- Loader 失败策略；
- Diagnostic 切片边界；
- 当前实现状态；
- 下一步 Browser Call。

## 7. 非目标

本次未实现：

- Browser Capability Call；
- NDJSON Event/Response；
- `host.ui.state` 和 `host.ui.command`；
- `pluginsdk.WebUI`；
- Diagnostic 运行时 Push；
- Settings Renderer；
- Plugins 页面；
- Session history/chat/trace；
- Agent Stream；
- 多浏览器同步；
- Event Replay；
- 插件启停；
- 插件 JavaScript 沙箱。

## 8. 已知风险

`go vet ./...` 当前失败：

```text
plugins/context-manager/main.go:33:
struct field messages has json tag but is not exported
```

该问题在 WebUI 切片前已存在。本次没有修改 `context-manager` 实现。

Loader 测试依赖 Node。没有 Node 的环境会跳过 JavaScript 测试。当前环境使用 Node `v24.19.0`。

## 9. 下一步

进入切片 3：Browser Call。

建议顺序：

1. 定义 Browser Call 的 NDJSON Envelope；
2. 新增 `POST /bridge/call`；
3. 新增 Host `CallWithEvents` 外部入口；
4. 在 `static/lite-agent.js` 提供 `LiteAgent.call`；
5. 验证 Event、Response、断开和不可伪造 `from`。

切片 3 不同时实现 UI Push 和 Session 前端。
## 10. 切片 3 逐步实施

### 10.1 Host CallWithEvents

状态：已完成。

交付：

- 新增 `internal/host/call.go`；
- 新增 `CallWithEvents` 外部调用入口；
- 复用 `WaitHost`、pending 和死亡闭环；
- 新增 `internal/host/call_test.go`。

验证：

- `go test ./...` 通过；
- `go test -race ./internal/host` 通过；
- 成功 Response、Event 顺序、Event 身份和 payload 均已覆盖；
- `host.*`、未知路由、非法参数和插件退出均已覆盖；
- `onEvent=nil` 路径已覆盖。

本步骤没有修改 HTTP、Browser Wire 和浏览器 SDK。
### 10.2 Browser Wire Envelope

状态：已完成。

交付：

- 新增 `internal/webui/wire.go`；
- 定义 Browser Call Request；
- 校验版本、ID、类型、capability 和 method；
- 拒绝 `from` 和未知字段；
- 将空 payload 规范化为 `{}`；
- 提供 Event/Response 的浏览器 ID 重写；
- 新增 `internal/webui/wire_test.go`。

验证：

- `go test ./...` 通过；
- `go test -race ./internal/webui` 通过；
- 合法请求、默认 payload、非法字段和尾随 JSON 均已覆盖；
- ID 重写不修改 Host 原始 Frame。

本步骤没有接入 HTTP 路由。
### 10.3 POST /bridge/call NDJSON

状态：已完成。

交付：

- 新增 `internal/webui/bridge.go`；
- `NewHandler` 接收窄接口 `CallBackend`；
- 新增 `POST /bridge/call`；
- 输出 `application/x-ndjson`；
- Event 和 Response 重写为浏览器请求 ID；
- 使用可信 `from="webui"`；
- 拒绝 `host.*`；
- 调用错误映射为稳定错误码；
- 非法请求体返回 `400`；
- Event 使用非阻塞有界队列。

验证：

- `go test ./...` 通过；
- `go test -race ./internal/webui ./internal/host` 通过；
- `go build ./cmd/liteagent-server` 通过；
- 实际调用 `session.list` 返回一条 NDJSON Response；
- `host.plugins` 和未知路由返回稳定错误 Response；
- 带 `from` 的请求返回 `400`。

本步骤没有实现 `LiteAgent.call`、UI Push 和断开重连。
### 10.4 Browser Call 与 UI Push 收口

状态：已完成。

交付：

- `LiteAgent.call` 实现 Request、NDJSON Event 和 Response 闭环；
- `LiteAgent.onUI`、`LiteAgent.onDiagnostic`；
- `Plugin.WebUI().SetState`；
- `Plugin.WebUI().SendCommand`；
- `host.ui.state`、`host.ui.command`；
- `/bridge/ui` State、Command 和 Diagnostic Push；
- Mount 属主校验；
- 单活动 UI 连接、FIFO 有界队列和断线丢弃。

遇到的问题与解决方法：

1. **Event 回调可能阻塞 Host 读取循环。**  
   Browser Bridge 使用有界队列，Event 非阻塞入队；最终 Response 使用有序等待，保证 Event 不会越过 Response。

2. **浏览器 ID 与 Host pending ID 不同。**  
   Browser Bridge 保留浏览器请求 ID，Host 内部继续使用 `host-N`，输出前统一重写 ID。

3. **UI Publisher 为空时，方法值取用会 panic。**  
   Host 在 `hostUIPublish` 中先读取 Publisher，再按方法分派，缺失时返回 `ui_not_mounted`。

4. **UI 流建立时客户端会等待首个输出。**  
   `/bridge/call` 和 `/bridge/ui` 都在响应头后立即 Flush，浏览器可以尽早开始读流。

5. **JSON 尾随数据的 EOF 类型判断错误。**  
   使用 `io.EOF` 判断单对象结束，避免尾随 JSON 被误接受。

验证：

- `go test ./...` 通过；
- `go test -race ./internal/host ./internal/webui ./pluginsdk` 通过；
- Node Browser SDK 与 Loader 测试共 7 项通过；
- 断开请求不会取消后端 Call；
- UI Stream 可接收 State、Command 和 Diagnostic；
- `host.ui.state`、`host.ui.command` 使用可信插件 `from`；
- 跨插件 Mount 返回 `ui_owner_mismatch`；
- 未挂载 Mount 返回 `ui_not_mounted`。

剩余范围：

- Settings Renderer；
- Plugins 页面；
- 插件生命周期管理；
- 更完整的慢客户端和重连压力测试。
