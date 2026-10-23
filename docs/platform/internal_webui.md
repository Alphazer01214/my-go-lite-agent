# internal/webui

`internal/webui` 是 Host 的浏览器展示面。它是 Host 与浏览器之间的唯一通道。

浏览器侧的线契约见 [../webui.md](../webui.md)。本文只讲实现。

## 1. 职责边界

本包负责：

- Mount Registry 与插件 UI 资源解析；
- Bootstrap 快照；
- Browser Call 的 NDJSON 闭环；
- UI Push 队列与 Diagnostic 推送；
- 插件扫描与启停接口；
- 系统文件夹选择器；
- Host Shell 静态资源服务。

本包不负责：

- 业务逻辑。业务在插件里；
- 路由表。路由表在 `internal/host`；
- 浏览器端渲染逻辑。渲染逻辑在 `internal/webui/static`。

本包 import `internal/host`，用的是 `host.PluginSnapshot`、`host.ErrorCode`、`host.ErrMethodNotFound`、`host.ErrBadPayload`、`host.ErrPluginNotMounted`、`host.FromHost` 和 `host.ProviderStateInactive`。它不修改 Host 的内部状态。

## 2. 文件与职责

| 文件 | 职责 |
|---|---|
| `server.go` | HTTP 路由、安全响应头、静态资源服务、连接管理 |
| `registry.go` | Mount Registry、UI 入口解析、插件资源解析 |
| `bootstrap.go` | Bootstrap 数据结构 |
| `bridge.go` | Browser Call 的 NDJSON 闭环与诊断生成 |
| `wire.go` | 浏览器请求与响应的编解码 |
| `publisher.go` | UI Push 队列、Diagnostic 推送与保留 |
| `management.go` | 插件扫描与启停接口 |
| `plugin_snapshot.go` | 静态快照与运行期快照合并 |
| `picker.go` | 系统文件夹选择器 |

浏览器侧脚本在 `internal/webui/static/`：

| 文件 | 职责 |
|---|---|
| `index.html` | Shell 的 DOM 结构与五个 Slot |
| `app.js` | Shell 逻辑：Bootstrap、UI Push 分发、诊断面板、插件中心编排 |
| `lite-agent.js` | 浏览器 SDK，导出单例 `LiteAgent` |
| `loader.mjs` | Component Loader |
| `plugins.mjs` | 插件中心的渲染函数 |
| `settings.mjs` | Settings Renderer 的渲染函数 |
| `shell.css` | Shell 样式 |

静态资源通过 `go:embed` 编进 `liteagent-server`，不落成独立文件。

## 3. Bootstrap

```go
type Bootstrap struct {
    Version     int
    Slots       []string
    Plugins     []BootstrapPlugin
    Mounts      []BootstrapMount
    Diagnostics []plugin.Diagnostic
}

type BootstrapPlugin struct {
    host.PluginSnapshot
    Settings *plugin.SettingsManifest
    Mounts   []MountRef
}

type BootstrapMount struct {
    Mount MountRef
    Tag   string
    Slot  string
    Entry string
}

type MountRef struct {
    Plugin string `json:"plugin"`
    ID     string `json:"id"`
}

type MountKey struct {
    Plugin string
    ID     string
}
```

`Bootstrap.Version` 固定是 1。`Slots` 固定是 `top`、`left`、`center`、`right`、`bottom`。

`BootstrapPlugin` 内嵌 `host.PluginSnapshot`。`Settings` 和 `Mounts` 在内嵌结构之外，因此快照合并时它们不会被运行期状态覆盖。

### 3.1 快照合并

`mergePluginSnapshots(discovered, runtime)` 的规则：

1. 先放静态快照。同名只保留第一个。
2. 再放运行期快照。同名时用运行期的 `PluginSnapshot` 整体覆盖静态的。
3. 运行期独有的插件追加到列表。
4. 最后按插件名排序。

覆盖只作用于内嵌的 `PluginSnapshot` 字段。`Settings` 和 `Mounts` 保留静态值。

没有运行期 provider 时，静态快照原样返回，状态是 `inactive`。

## 4. Mount Registry

### 4.1 结构

```go
type Registry struct {
    mounts        map[MountKey]MountSpec
    order         []MountKey
    plugins       map[string]BootstrapPlugin
    pluginOrder   []string
    resourceRoots map[string]string
    pluginRoots   map[string]string
    diagnostics   []plugin.Diagnostic
}

type MountSpec struct {
    Key      MountKey
    Tag      string
    Slot     string
    EntryURL string
}
```

`Registry` 构造后不可变。`replaceRegistry` 发布的是新指针，旧读者继续用旧指针跑完。

没有 `Replace` 方法。原地改内容会让并发读者看到半写的 map。

### 4.2 构建

`NewRegistry(discoveries)` 返回 `(*Registry, []Diagnostic)`。

1. 插件名重复时记 `ui_plugin_duplicate` 并跳过。
2. 没有 `ui` 段的插件不进 Mount，但仍然进插件列表。
3. 解析 `ui.entry`。解析失败时该插件贡献零个 Mount，插件本身保留。
4. 解析成功时记下两个根：
   - `pluginRoots[plugin]` 是插件目录；
   - `resourceRoots[plugin]` 是 `ui.entry` 所在目录。
5. 逐个校验 Component。合法的按 Manifest 顺序进 `order`。
6. 插件列表按名字排序。

`resourceRoots` 存的是符号链接解析后的目录。

### 4.3 UI 入口解析

`resolveEntry(pluginDir, entry, pluginName, sourcePath)` 的规则：

| 情形 | 诊断码 |
|---|---|
| entry 为空 | `ui_entry_invalid` |
| entry 是绝对路径或含 `..` 段 | `ui_entry_outside_package` |
| entry 指向的文件不存在 | `ui_entry_missing` |
| entry 不是常规文件 | `ui_entry_invalid` |
| entry 解析到插件包外 | `ui_entry_outside_package` |
| 插件目录不可用 | `ui_resource_root_invalid` |
| 符号链接解析失败 | `ui_entry_outside_package` |

相对路径的判定：非空、不含 `..` 段、不是绝对路径。

符号链接解析后仍在插件目录内才算合法。

### 4.4 Component 校验

| 字段 | 规则 | 诊断码 |
|---|---|---|
| `tag` | 匹配 `^[a-z][a-z0-9._-]*-[a-z0-9._-]+$` | `ui_tag_invalid` |
| `tag` | 以 `<plugin_name>-` 开头且不等于它 | `ui_tag_prefix_mismatch` |
| `mount_id` | 匹配 `^[a-z0-9_-]+$` | `ui_mount_id_invalid` |
| `mount_id` | 插件内唯一 | `ui_mount_duplicate` |
| `slot` | `top` 被 Host 保留 | `ui_slot_reserved` |
| `slot` | 只能是 `left`、`center`、`right`、`bottom` | `ui_slot_invalid` |

任一条不通过，这个 Component 被整体丢弃。其它 Component 不受影响。

同一个 slot 可以挂多个 Mount。`mount_id` 在插件内必须唯一。

### 4.5 资源解析

`ResolveResource(pluginName, packagePath)` 的判定顺序：

1. 插件没有 `resourceRoots` 时不解析。
2. 包路径含 `..` 段时不解析。
3. 拼出候选路径，符号链接解析后必须仍在 `resourceRoots` 内。
4. 必须是常规文件。

结果是文件的真实绝对路径。`plugin.json` 这类插件根目录下的文件不在资源根内，因此不解析。

`ResourceRoot(pluginName)` 返回资源根。

## 5. Browser Call

### 5.1 浏览器请求

```go
type browserCall struct {
    Version    int
    ID         string
    Type       string
    Capability string
    Method     string
    Payload    json.RawMessage
}
```

`decodeBrowserCall` 的校验：

| 情形 | 错误消息 |
|---|---|
| 出现未知字段 | JSON 解码错误，指出字段名 |
| `from` 出现，含 `null` | `browser call from must not be provided` |
| body 里出现第二个 JSON 值 | `browser call must contain one JSON object` |
| body 不是 JSON 对象 | JSON 解码错误 |
| `version` 不是 1 | `browser call version must be 1` |
| `id` 为空 | `browser call id is required` |
| `type` 不是 `req` | `browser call type must be "req"` |
| `capability` 为空 | `browser call capability is required` |
| `method` 为空 | `browser call method is required` |

解码器用 `DisallowUnknownFields`。`from` 单独用 `json.RawMessage` 接收，因此 `"from": null` 也算提供了。

payload 为空或 `null` 时归一为 `{}`。其余情况做一次压缩。

### 5.2 浏览器应答

`browserFrame(source, requestID)` 把后端帧转成浏览器帧：

- `version` 改写为 1；
- `id` 改写为浏览器的请求 id；
- 源帧不被修改；
- 其它字段原样保留。

只接受 `evt` 和 `res` 两种帧类型。源帧为 nil 或请求 id 为空时报错。

### 5.3 闭环

`serveBrowserCall` 的流程：

1. 后端为 nil 时返回 500。
2. 请求体上限是 `protocol.FrameMaxSize`。
3. 解码请求。失败返回 400，后端不被调用。
4. `capability` 是 `host` 时直接返回一条 `method_not_found` 应答，后端不被调用。
5. 响应头设为 `application/x-ndjson`，禁止缓冲，立刻 flush。
6. 起一个 goroutine 跑后端调用。
7. 主循环从队列取帧，逐条编码并 flush。收到 `res` 时结束。

后端调用的 Context 用 `context.WithoutCancel(r.Context())` 构造。浏览器断开不取消业务调用。

队列容量是 64。事件满了就丢，事件是进度信号。终态应答不丢，它会阻塞到写出去或浏览器离开。

帧转不出来的场合合成一条错误应答。不存在静默丢弃。

### 5.4 诊断生成

`callDiagnostic(request, code, message)` 的规则：

| 情形 | 是否产生诊断 |
|---|---|
| `code` 为空 | 否 |
| `code` 是 `call_cancelled` | 否 |
| 其它 | 是 |

诊断的 `Plugin` 取请求的 capability，`Path` 取 `capability.method`。

成功的调用不产生诊断。用户显式取消不产生诊断。

## 6. UI 推送

### 6.1 包络

```go
type uiEnvelope struct {
    Version    int
    ID         string
    Type       string
    Mount      *MountRef
    Kind       string
    Command    string
    Payload    json.RawMessage
    Diagnostic *plugin.Diagnostic
}
```

| 场景 | `type` | `kind` |
|---|---|---|
| State | `ui` | `state` |
| Command | `ui` | `command` |
| Diagnostic | `diagnostic` | 无 |
| 连接建立 | `hello` | 无 |

`version` 固定是 `protocol.FrameVersion`，即 2。这与 Browser Call 的版本 1 不同。

id 前缀：`ui-push-N` 用于推送和 hello，`diag-N` 用于诊断。

### 6.2 连接

```go
type uiConnection struct {
    id           string
    queue        chan *uiEnvelope
    drops        atomic.Uint64
    overflow     chan struct{}
    onOverflow   func()
    overflowOnce sync.Once
}
```

每条连接的队列容量是 64。

`serveUI` 建立连接后：

1. 分配 `ui-conn-N` 形式的 id。
2. 先发一条 hello。hello 一定排在已入队的推送前面。
3. 循环取帧编码并 flush。
4. 收到 `overflow` 信号时记日志并退出，关闭这条连接。
5. 客户端断开时退出。

`onOverflow` 把写超时设到过去，让卡在慢 socket 上的 writer 立刻失败退出。

连接之间互不影响。新连接不挤掉旧连接。

### 6.3 扇出

`enqueueUI` 把一条包络投给所有活动连接。

- 至少一个连接接下就算投递成功。
- 某个连接队列满时，它的 `drops` 加一，并触发 `overflow`。其它连接不受影响。

投递成功时 `PublishState` 和 `PublishCommand` 返回 `{}`。

没有连接接下时返回错误，包装 `protocol.ErrUINotMounted`，消息是 `no ui connection for mount <id>`。

### 6.4 Mount 属主

`resolveMount(from, mountID)`：

1. `{Plugin: from, ID: mountID}` 在注册表里时命中。
2. 其它插件拥有同名 `mount_id` 时返回 `protocol.ErrUIOwnerMismatch`。
3. 都不匹配时返回 `protocol.ErrUINotMounted`。

`from` 是发起方的 Plugin Instance id。默认实例的 id 等于插件名，因此能命中 Mount。带实例后缀的次要实例发布时命中不了。

### 6.5 发布校验

`PublishState` 与 `PublishCommand` 用同一个严格解码器。它拒绝未知字段和多对象输入。

| 情形 | 错误 |
|---|---|
| payload 解不出 `mount_id` 和 `state` | 包装 `host.ErrBadPayload` |
| 外层 payload 为空 | 包装 `host.ErrBadPayload` |
| `mount_id` 为空 | 包装 `host.ErrBadPayload` |
| `state` 为空 | 包装 `host.ErrBadPayload` |
| `command` 为空 | 包装 `host.ErrBadPayload` |
| command 的内层 `payload` 为空或 `null` | 归一为 `{}` |

`state` 字节原样转发，不重新压缩。`state` 为空时不归一，直接判为 `bad_payload`。

### 6.6 Diagnostic 保留

`PushDiagnostic` 把诊断追加进内存环形缓冲，上限 256 条。超出后丢弃最旧的。

诊断同时作为 `diagnostic` 包络推给所有连接。

环形缓冲的内容会出现在每次 Bootstrap 的 `diagnostics` 字段里。

## 7. 管理接口

`PluginLifecycle` 是本包与 Host 之间的窄缝：

```go
type PluginLifecycle interface {
    PluginSnapshot() []host.PluginSnapshot
    RescanPlugins() (plugin.Discoveries, error)
    StartPlugin(name string) error
    StopPlugin(name string) error
}
```

`Server.SetPluginManager` 同时接上这个接口和 `PluginStateProvider`。

### 7.1 端点

| 端点 | 行为 |
|---|---|
| `POST /bridge/plugins/rescan` | 重新扫描，替换 Registry，返回最新 Bootstrap |
| `POST /bridge/plugins/{plugin}/start` | 启动插件，返回最新 Bootstrap |
| `POST /bridge/plugins/{plugin}/stop` | 停止插件，返回最新 Bootstrap |

三个端点都返回完整 Bootstrap，不是状态对象。

### 7.2 错误

| 码 | HTTP | 含义 |
|---|---|---|
| `plugin_manager_unavailable` | 503 | 没有接上插件管理器 |
| `plugin_rescan_failed` | 400 | 扫描失败 |
| `plugin_action_failed` | 400 | 启停失败 |
| `bad_arguments` | 400 | 路径里没有插件名 |

插件未被发现时，启停失败返回 404。

错误响应体：

```json
{"error": {"code": "plugin_action_failed", "message": "..."}}
```

## 8. 文件夹选择器

`servePickDirectory` 是 Host 平台能力，不经过插件 Capability。

结果结构：

```go
type pickDirectoryOut struct {
    Path      string `json:"path,omitempty"`
    Cancelled bool   `json:"cancelled,omitempty"`
}
```

| 情形 | HTTP | 码 |
|---|---|---|
| 选中了目录 | 200 | 无。返回 `{"path": "..."}` |
| 用户取消 | 200 | 无。返回 `{"cancelled": true}` |
| 平台不支持 | 501 | `folder_picker_unsupported` |
| 选择器执行失败 | 500 | `folder_picker_failed` |
| 浏览器在对话框打开期间离开 | 无响应 | 无 |

`pickDirectoryNative` 按 `runtime.GOOS` 分派：

| 平台 | 工具 |
|---|---|
| macOS | `osascript` 调 `choose folder` |
| Windows | `powershell.exe` 调 `FolderBrowserDialog` |
| Linux | `zenity`，不可用时试 `kdialog` |
| 其它 | 返回 `errDirectoryPickerUnsupported` |

Windows 的输出用 base64 传回，避免路径里的非 ASCII 字符被控制台编码改写。

Linux 上 `zenity` 退出码 1 表示用户取消。`kdialog` 不可用时返回不支持。

`pickerResult` 要求绝对路径。相对路径报错。

## 9. HTTP 层

### 9.1 路由

| 方法 | 路径 | 处理 |
|---|---|---|
| `GET` | `/{$}` | Shell 的 `index.html` |
| `GET` | `/platform/{path...}` | Shell 自身的 JS 和 CSS |
| `GET` | `/bridge/bootstrap` | Bootstrap 快照 |
| `POST` | `/bridge/call` | Browser Call |
| `POST` | `/bridge/pick-directory` | 文件夹选择器 |
| `GET` | `/bridge/ui` | UI Push 流 |
| `POST` | `/bridge/plugins/rescan` | 重新扫描 |
| `POST` | `/bridge/plugins/{plugin}/start` | 启动插件 |
| `POST` | `/bridge/plugins/{plugin}/stop` | 停止插件 |
| `GET` | `/plugins/{plugin}/{path...}` | 插件 UI 资源 |

只有 `GET` 的路由收到 `POST` 时返回 405。

路径含 `..` 段时直接返回 404。这个检查在安全头之前，因此那个 404 不带安全头。

### 9.2 安全头

| 头 | 值 |
|---|---|
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `no-referrer` |
| `Cache-Control` | `no-cache` |
| `Content-Security-Policy` | `default-src 'self'` 等 |

CSP 完整值：

```text
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
img-src 'self' data:; connect-src 'self'; object-src 'none';
base-uri 'none'; frame-ancestors 'none'
```

### 9.3 同源检查

`sameOriginHandler` 包装所有会改状态的 `POST` 端点。

带 `Origin` 且 host 与请求 host 不一致时返回 403。不带 `Origin` 的请求放行。

`Origin` 解析失败时视为跨源，返回 403。

### 9.4 Content-Type

`setContentType` 按扩展名映射：

| 扩展名 | 类型 |
|---|---|
| `.js`、`.mjs` | `text/javascript; charset=utf-8` |
| `.css` | `text/css; charset=utf-8` |
| `.html` | `text/html; charset=utf-8` |
| `.json` | `application/json; charset=utf-8` |
| 其它 | 走 `mime.TypeByExtension` |

不认识的扩展名不设 Content-Type。

### 9.5 资源服务

`servePluginResource` 委托给 `Registry.ResolveResource`。解析失败、文件打不开或不是常规文件时返回 404。

`serveEmbeddedAsset` 读 `go:embed` 的静态资源。清理后的路径为空、`.` 或 `..` 时返回 404。

## 10. 浏览器侧脚本

### 10.1 LiteAgent SDK

`lite-agent.js` 导出单例 `LiteAgent` 和工厂 `createLiteAgent(options)`。

| 选项 | 默认值 |
|---|---|
| `fetchImpl` | 全局 `fetch` |
| `newRequestID` | `ui-call-<uuid>` |
| `reconnectDelayMs` | 1000 |
| `maxReconnectDelayMs` | 10000 |

| 方法 | 语义 |
|---|---|
| `call(capability, method, payload, onEvent)` | 发起 Capability Call |
| `pickDirectory()` | 打开文件夹选择器 |
| `onUI(handler)` | 订阅 State 和 Command |
| `onDiagnostic(handler)` | 订阅 Diagnostic |
| `onStatus(handler)` | 订阅连接状态 |
| `onResync(handler)` | 订阅重连信号 |
| `closeUI()` | 关闭 UI 流并清空订阅 |

失败时 reject 的 Error 带 `code`。`code` 取自 `error_code`，缺省 `handler_error`。Error 上还有 `response` 指向完整应答帧。

`onStatus` 注册后立即以当前状态调用一次。状态只有 `connecting`、`connected`、`reconnecting` 三个取值。

重连退避是 `1s → 2s → 4s → 8s → 10s → 10s → ...`。收到 hello 时计数清零。没有次数上限，没有随机抖动。

### 10.2 Component Loader

`loader.mjs` 导出 `mountKeyFor(mount)` 和 `createMountLoader(options)`。

| 选项 | 默认值 |
|---|---|
| `importModule` | 必填 |
| `customElements` | 必填 |
| `document` | 必填 |
| `slots` | 必填 |
| `onDiagnostic` | 可选 |
| `whenDefinedTimeoutMs` | 10000 |
| `resyncRetryDelaysMs` | `[0, 250, 500, 1000, 2000]` |

Mount 键是 `plugin:id`。

`load(mounts)` 是对齐而不是追加。不在新列表里的元素被移除。共享同一个 `entry` 的 Mount 只导入一次。

导入失败的 entry 不缓存，下次 `load()` 会重试。

`resync(mounts)` 同样做移除对齐。组件实现了 `resync()` 就调用它，失败时按退避重试。没有实现就移除元素重新挂载。

Loader 只发两种诊断码：

| 码 | 含义 |
|---|---|
| `ui_mount_failed` | 装载失败。导入失败、标签定义超时或插槽未知 |
| `ui_resync_failed` | `resync()` 重试后仍失败 |

诊断的 `Path` 是 UI 入口 URL，`Plugin` 是插件名。

Loader 不调用 `setData` 和 `runCommand`。这两个方法由 Shell 调用。

### 10.3 Shell

`app.js` 的职责：

- 读 Bootstrap，渲染插件中心、依赖图和选中插件的设置；
- 建 Mount Loader 并按 Bootstrap 装载 Mount；
- 把 UI Push 分发到对应组件的 `setData` 和 `runCommand`；
- 渲染诊断面板；
- 响应重连并触发 resync。

UI Push 分发的规则：

- 帧没有 `mount` 时忽略。
- 找不到对应组件时忽略。
- `kind` 是 `state` 时调 `setData(payload)`。
- `kind` 是 `command` 时调 `runCommand(command, payload)`。
- 其它 `kind` 忽略。
- 组件没有对应方法时忽略。

组件抛出的异常不被捕获，会中断 UI 流并触发重连。

诊断合并键是 `plugin`、`path`、`code` 三者的组合。相同键的条目合并并计数。

Shell 的诊断来源分两路：服务端 Bootstrap 的诊断和浏览器本地产生的诊断。服务端诊断在每次 Bootstrap 时整体替换。

### 10.4 插件中心与设置

`plugins.mjs` 和 `settings.mjs` 是纯渲染模块。它们不发请求。插件中心使用原生 `dialog` Drawer，图区与详情并排，窄屏上下排列。

| 导出 | 职责 |
|---|---|
| `renderDependencyGraph` | 依赖关系 SVG 图 |
| `renderPluginDetail` | 选中插件的状态、启停、实例、关系，以及设置挂载点；无选中时渲染概览 |
| `renderDependencyEdgeList` | 关系的文本列表 |
| `renderSettingsForm` | 单插件设置表单；不可用时只渲染提示 |

依赖图的模型：

- 节点是所有插件名，加上只出现在 `depends_on` 里的名字。
- 深度是 `1 + max(依赖深度)`。有环时按 0 处理，不挂死。
- `depends` 边从依赖方指向被依赖方。
- `mount` 边从 Host 指向非缺失的插件。
- 缺失节点以 `missing` 状态显示。
- 选中节点时，它和直接相连的节点高亮，其余淡出。
- 点击节点保留选中；点击画布空白或详情里的「清除选择」取消选中。

Shell 只对当前选中插件调用 `settings.get`。设置表单挂在 `renderPluginDetail` 返回的 `settingsRoot` 上；插件不可用时该区域只渲染提示。切换实例时 Shell 复用已有表单节点，不重新读取设置。

设置表单的取值规则：

| 字段类型 | 收集规则 |
|---|---|
| `string`、`select` | 非 `null` 时按字符串收集。空串照常提交 |
| `secret` | 只在非空时收集。空串不出现在 payload 里 |
| `number` | 非空且是有限数时按数字收集 |
| `boolean` | 总是收集，按勾选状态取布尔值 |
| `list` | 总是收集为数组。整行无值的行被丢弃 |

`options_from` 的选项来自被引用 `list` 字段在页面上的当前行，不是已保存的值。`option_value` 默认 `id`，`option_label` 默认 `label`。

`item_fields` 的 `select` 类型带一个空占位项，保证新加的空行不会被提交。

## 11. 测试覆盖

| 领域 | 覆盖情况 |
|---|---|
| 请求解码 | 已覆盖 9 种非法输入。只断言关键词，错误消息原文未固定 |
| 应答转换 | 已覆盖 id 改写、源帧不变、三种拒绝 |
| Call 闭环 | 已覆盖事件在前应答在后、非法 body 不触达后端、`host` capability 被拒、错误应答、无事件时只有一条应答 |
| 诊断生成 | 已覆盖取消不产生诊断、空码不产生诊断、插件错误码产生诊断、非哨兵错误映射为 `handler_error` |
| 断线 | 已覆盖浏览器断开不取消后端调用 |
| 发布 | 已覆盖包络字段、属主校验、payload 校验、扇出、队列满时的隔离 |
| 无连接 | 已覆盖发布不阻塞并返回错误。实际走的是 Mount 找不到分支 |
| Registry | 已覆盖 Mount 构建、入口 URL、资源根、路径穿越拒绝、第 4.4 节六条校验码中的四条、坏 entry 不占位 |
| 扫描诊断 | 已覆盖发现期诊断进入 Bootstrap |
| HTTP 路由 | 已覆盖 Shell、平台资源、Bootstrap 的状态码与 Content-Type、插件资源 404 边界、405 |
| 路径泄漏 | 已覆盖 Bootstrap 不含本地路径 |
| 管理接口 | 已覆盖 rescan、start、stop 返回 Bootstrap，失败返回 400 和结构化错误 |
| 文件夹选择器 | 已覆盖成功、取消、两种错误码、跨源 403、Windows base64 解码 |
| UI 流 | 已覆盖 hello 优先、状态推送、诊断推送、多连接独立 |
| 快照合并 | 已覆盖运行期覆盖静态、运行期独有插件追加、按名排序、静态兜底 |

未被测试固定的行为：

- Bridge 的背压差异。事件在 64 帧处丢弃、终态应答不丢，没有测试填满队列。
- Registry 的不可变性。
- 符号链接指向 UI 根之外的拒绝。
- `nosniff` 之外的安全头。
- 跨源检查在 `/bridge/call` 和 `/bridge/plugins/*` 上的效果。
- 管理接口的 503 与 404 分支。
- 选择器的分平台分派和相对路径拒绝。
- Diagnostic 环形缓冲的 256 上限。
- UI Push 包络的 JSON 键名和 id 前缀。
- `ui_tag_invalid` 和 `ui_slot_invalid` 两个 Registry 诊断码。
- `no ui connection for mount <id>` 这条无连接分支。
- 请求解码各类错误的错误消息原文。
