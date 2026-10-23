# WebUI

WebUI 是 Host 的平台展示面。Host 提供 Shell、Slot、Loader 和 Browser Bridge。插件提供自己的 Custom Element。Plugin Runtime 驱动自己拥有的 Component。

组件业务含义由插件文档定义。本文只定义 Host 与插件前端之间的平台接口。

## 1. 技术栈

```text
HTML
CSS Variables
ES Modules
Custom Elements
Shadow DOM
原生 DOM API
Go HTTP / embedded FS
自研 Web Bridge
```

v1 不引入浏览器框架、npm、打包器和 iframe。

## 2. 组成

```mermaid
flowchart TB
    subgraph Host[Host WebUI]
        Shell[Shell]
        Slots[Slot Registry]
        Loader[Component Loader]
        Settings[Settings Renderer]
        Bridge[Browser Bridge]
    end
    subgraph Plugin[Plugin Runtime]
        State[State]
        Command[Command]
        Call[Capability Call]
    end
    subgraph UI[Plugin Component]
        Render[State 展示]
        Action[Command 响应]
        Up[Capability Call]
    end
    Loader --> UI
    State --> Bridge
    Command --> Bridge
    Bridge --> UI
    Up --> Bridge
    Bridge --> Call
```

Host 侧的代码分布在 `internal/webui`：

| 文件 | 职责 |
|---|---|
| `server.go` | HTTP 路由、安全响应头、静态资源服务 |
| `registry.go` | Mount Registry 与插件资源解析 |
| `bootstrap.go` | Bootstrap 数据结构 |
| `bridge.go` | Browser Call 的 NDJSON 闭环 |
| `wire.go` | 浏览器侧请求与响应的编解码 |
| `publisher.go` | UI Push 队列与 Diagnostic 推送 |
| `management.go` | 插件扫描与启停接口 |
| `plugin_snapshot.go` | 静态快照与运行期快照合并 |
| `picker.go` | 系统文件夹选择器 |

## 3. Component Interface

每个 Component 统一实现：

```js
setData(state)
runCommand(command, payload)
connectedCallback()
disconnectedCallback()
```

可选实现 `resync()`。UI 连接重连后，Shell 调用已挂载组件的 `resync()` 重新读取状态；组件未实现时，Shell 重新挂载组件，由 `connectedCallback` 重新取数。

State 决定 Component 当前是什么。Command 决定 Component 此刻做什么。两类 payload 都由插件定义。

Component 在 `connectedCallback` 中通过 Capability Call 获取初始数据。Plugin Runtime 后续使用 `SetState` 更新。Component 不实现独立的业务 API。

WebUI 只负责交互和展示。业务执行、业务状态和持久化都位于后端 Plugin Runtime。浏览器不是应用状态属主。

`setData` 与 `runCommand` 由 Shell 调用，不由 Loader 调用。Loader 只负责创建元素、挂载元素和调用 `resync()`。

组件抛出的异常不会被捕获。它会中断 UI 流的读取，触发 SDK 重连。

## 4. Mount

静态 Mount 来自 `plugin.json.ui.components`。

```text
MountKey = plugin_name + mount_id
```

Slot 为：

```text
top
left
center
right
bottom
```

Host Shell 只决定布局位置。Host 不解释 Component 的业务内容。v1 的 `top` 由 Host 控件保留，插件使用其余四个 Slot。

`mount_id` 只定位实例，不参与业务路由。插件继续使用自己的业务 id 关联记录。

Loader 按 `entry` 去重加载。多个 Mount 共享同一个 `entry` 时只导入一次。

单个 entry 导入失败或标签定义超时，只跳过对应 Mount。失败会写入本地 Diagnostic，其他 Mount 继续加载。

失败的 entry 不会被缓存，下次 `load()`（即下一次 bootstrap 或 rescan）会重新尝试，不需要刷新页面。

`load()` 是对齐而不是追加：不在新 Mount 列表里的元素会被移除，Slot 不会残留已经下线的 Mount。

`resync()` 的行为：

| 情况 | 处理 |
|---|---|
| 组件实现了 `resync()` | 调用它。失败时按 0、250、500、1000、2000 毫秒重试 |
| 组件没有 `resync()` | 移除元素并重新挂载，触发 `connectedCallback` |
| 组件不存在 | 按普通装载处理 |

`resync()` 重试全部失败后写一条 `ui_resync_failed` 诊断，Mount 保留在页面上。

## 5. pluginsdk WebUI

目标 Go Interface：

```go
func (u *WebUI) SetState(mountID string, state json.RawMessage) error
func (u *WebUI) SendCommand(mountID string, command string, payload json.RawMessage) error
```

内部目标地址由 Host 生成：

```text
发起方实例 id + mountID
```

插件不能向其他插件的 mount 发送 State 或 Command。

发起方实例 id 对默认实例等于插件名。带实例后缀的次要实例发布时会因为找不到 Mount 而被拒。

`SetState` 对应：

```js
component.setData(state)
```

`SendCommand` 对应：

```js
component.runCommand(command, payload)
```

## 6. State 和 Command

State 应可由 Plugin Runtime 重建。Command 可以是临时行为。

```json
{
  "revision": 12,
  "view": {},
  "status": "ready"
}
```

上面的 JSON 只是平台示例。`view` 的字段和 `status` 的取值由插件定义。

Command 示例：

```json
{
  "command": "scroll",
  "payload": {
    "position": "bottom"
  }
}
```

插件文档必须列出自己的 State、Command 和 payload。

## 7. Browser Bridge

v1 使用 HTTP 和 NDJSON 流：

```text
GET  /                    Host Shell
GET  /platform/...        Host Shell 自身的 JS/CSS
GET  /plugins/...         插件 UI 资源目录
GET  /bridge/bootstrap    Plugin、Mount、Slot 和 Diagnostic
POST /bridge/call         Capability Call
POST /bridge/pick-directory  选择本地文件夹
GET  /bridge/ui           State、Command 和 Diagnostic
POST /bridge/plugins/rescan
POST /bridge/plugins/{plugin}/start
POST /bridge/plugins/{plugin}/stop
```

服务默认监听 `127.0.0.1:2333`。`LITEAGENT_HTTP_ADDR` 可以覆盖监听地址。启动后输出实际 URL，不自动打开浏览器。监听失败时启动退出。

所有 `POST` 端点都要求同源：带 `Origin` 且 host 与请求不一致时返回 403。
不带 `Origin` 的请求（curl、测试）放行。这是为了让启停插件这类无 body、无自定义 content type 的接口不会被跨站表单触发。

### 7.1 版本号

WebUI 侧有两个独立的版本号。它们都不等于 Frame 版本。

| 场景 | 版本 | 来源 |
|---|---|---|
| Browser Call 请求与响应 | 1 | `webui.BrowserVersion` |
| Bootstrap | 1 | `webui.Bootstrap.Version` |
| UI Push、Diagnostic Push、hello | 2 | `protocol.FrameVersion` |

浏览器提交的 Call 版本必须是 1，其它值返回 400。

### 7.2 响应头

除路径含 `..` 时直接返回的 404 外，所有响应都带下列安全头：

| 头 | 值 |
|---|---|
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `no-referrer` |
| `Cache-Control` | `no-cache` |
| `Content-Security-Policy` | `default-src 'self'` 等，见下 |

```text
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
img-src 'self' data:; connect-src 'self'; object-src 'none';
base-uri 'none'; frame-ancestors 'none'
```

NDJSON 响应额外带 `X-Accel-Buffering: no`，避免代理缓冲。

URL 中出现 `..` 段时直接返回 404。

### 7.3 Bootstrap

```json
{
  "version": 1,
  "slots": ["top", "left", "center", "right", "bottom"],
  "plugins": [
    {
      "name": "example-plugin",
      "version": "1.0.0",
      "state": "active",
      "selected_instance_id": "example-plugin",
      "instances": [
        {
          "instance_id": "example-plugin",
          "generation": 1,
          "state": "active",
          "registrations": [{"capability": "example", "method": "run"}]
        }
      ],
      "enabled": true,
      "provides": ["example"],
      "depends_on": [],
      "mounts": [{"plugin": "example-plugin", "id": "main"}]
    }
  ],
  "mounts": [],
  "diagnostics": [
    {
      "plugin": "example-plugin",
      "path": "plugin.json",
      "code": "manifest_unknown_field",
      "message": "unknown field: pages"
    }
  ]
}
```

Bootstrap 返回插件状态、静态 Mount 和当前 Diagnostic 快照，不返回业务 State。

`plugins[]` 按插件名分组。`instances[]` 列出该插件的全部 Plugin Instance。Provider `state` 使用 `inactive`、`loading`、`active`、`stopping` 或 `failed`。

`selected_instance_id` 指出默认展示的实例。插件中心可以选择其他实例。`enabled` 表示插件是否允许启动；`missing` 表示依赖目标未发现。`provides`、`depends_on` 和 `mounts` 只返回静态 Manifest 信息。快照不包含插件目录或可执行文件路径。

`slots` 数组由 Host 固定给出。Shell 实际使用 DOM 中的槽位元素，不读这个数组。

静态快照与运行期快照合并时，同名插件以运行期状态覆盖静态状态。

### 7.4 Browser Request

```json
{
  "version": 1,
  "id": "ui-call-1",
  "type": "req",
  "capability": "example",
  "method": "run",
  "payload": {}
}
```

浏览器不提交 `from`。请求体里出现 `from` 字段时返回 400。

请求体上限是 16 MiB，与 Frame 上限一致。

`capability` 为 `host` 的请求直接被拒，返回 `method_not_found`。浏览器不能调用 Host 平台方法。

payload 为空或 `null` 时按 `{}` 处理。其余情况先做一次压缩再转发。

### 7.5 Response

```json
{
  "version": 1,
  "id": "ui-call-1",
  "type": "res",
  "capability": "example",
  "method": "run",
  "from": "example-plugin",
  "payload": {},
  "error_code": "",
  "error_msg": ""
}
```

`POST /bridge/call` 返回 NDJSON。零到多个归因 Event 后必须有一条 Response。

Event 可以被丢弃。Response 不被丢弃：它要么写出去，要么被合成为一条错误 Response。

浏览器断开只停止写回，不取消后端 Call。Host 继续等待后端唯一终态，并丢弃无法写入的 Event 和 Response。Host 不缓存结果，也不支持按 Call ID 重放。

取消业务必须调用插件显式定义的取消方法（例如 `loop.cancel`）；刷新或关闭页面不会隐式取消。

失败且不是显式取消的调用会额外产生一条 Diagnostic，`Plugin` 填 Capability，`Path` 填 `capability.method`。

### 7.6 目录选择

`POST /bridge/pick-directory` 打开 Host 所在机器的文件夹选择框，请求体为空。
该接口属于 Host 平台，不经过插件 Capability。
浏览器原生文件夹选择只返回句柄或相对路径，因此由 Host 返回绝对路径。
用户选中目录时返回：

```json
{"path": "/Users/example/project"}
```

用户取消时返回：

```json
{"cancelled": true}
```

Host 在 macOS、Windows 和 Linux 上调用系统选择器。
Linux 需要 `zenity` 或 `kdialog`。
平台不支持时返回 501，选择器执行失败时返回 500。
错误响应为 JSON，包含 `error.code` 和 `error.message`。
错误码为 `folder_picker_unsupported` 或 `folder_picker_failed`。
该接口与其它 `POST` 端点一样要求同源。

### 7.7 管理接口

扫描与启停接口返回最新 Bootstrap，供 Shell 更新插件快照和 Mount 列表。

成功时返回 200 和 Bootstrap 对象。

失败时返回 JSON 错误：

```json
{"error": {"code": "plugin_action_failed", "message": "..."}}
```

| 码 | HTTP | 含义 |
|---|---|---|
| `plugin_manager_unavailable` | 503 | 没有接上插件管理器 |
| `plugin_rescan_failed` | 400 | 扫描失败 |
| `plugin_action_failed` | 400 | 启停失败 |
| `bad_arguments` | 400 | 路径里没有插件名 |

插件未被发现时，启停失败返回 404。

### 7.8 Diagnostic Push

```json
{
  "version": 2,
  "id": "diag-1",
  "type": "diagnostic",
  "diagnostic": {
    "plugin": "example-plugin",
    "path": "ui/main.js",
    "code": "ui_mount_failed",
    "message": "Failed to fetch dynamically imported module"
  }
}
```

Diagnostic 与 UI Push 共用 `/bridge/ui` 连接和有界队列。它不要求 Mount。

### 7.9 UI State Push

```json
{
  "version": 2,
  "id": "ui-push-18",
  "type": "ui",
  "mount": {
    "plugin": "example-plugin",
    "id": "main"
  },
  "kind": "state",
  "payload": {
    "revision": 12
  }
}
```

### 7.10 UI Command Push

```json
{
  "version": 2,
  "id": "ui-push-19",
  "type": "ui",
  "mount": {
    "plugin": "example-plugin",
    "id": "main"
  },
  "kind": "command",
  "command": "scroll",
  "payload": {
    "position": "bottom"
  }
}
```

### 7.11 Browser SDK

SDK 是 `internal/webui/static/lite-agent.js` 导出的单例，名为 `LiteAgent`。工厂函数是 `createLiteAgent(options)`。

| 选项 | 默认值 | 含义 |
|---|---|---|
| `fetchImpl` | 全局 `fetch` | 替换底层请求实现 |
| `newRequestID` | `ui-call-<uuid>` | 生成 Call id |
| `reconnectDelayMs` | 1000 | 首次重连等待 |
| `maxReconnectDelayMs` | 10000 | 重连等待上限 |

| 方法 | 语义 |
|---|---|
| `call(capability, method, payload, onEvent)` | 发起 Capability Call，返回 Promise |
| `pickDirectory()` | 打开文件夹选择框，返回 Promise |
| `onUI(handler)` | 订阅 State 和 Command Push |
| `onDiagnostic(handler)` | 订阅 Diagnostic |
| `onStatus(handler)` | 订阅连接状态 |
| `onResync(handler)` | 订阅重连信号 |
| `closeUI()` | 关闭 UI 流并清空全部订阅 |

`capability` 和 `method` 必填，缺省抛 `TypeError`。`onEvent` 必须是函数。

`call` 成功时 resolve 最终 Response 的 `payload`。失败时 reject 一个带 `code` 的 Error，`code` 取自 `error_code`，缺省为 `handler_error`。Error 上还有 `response` 字段，指向完整的 Response 帧。

SDK 自身抛出的错误没有 `code`，例如 HTTP 非 2xx、帧 id 不匹配、帧类型不对、流结束前没有 Response。

`pickDirectory` 成功时 resolve `{path}` 或 `{cancelled:true}`。失败时 reject 带 `code` 的 Error，缺省码为 `folder_picker_failed`。

`onStatus` 注册后立即以当前状态调用一次 handler，之后只在状态变化时调用。状态取值只有三个：`connecting`、`connected`、`reconnecting`。

`onUI`、`onDiagnostic`、`onStatus`、`onResync` 都不返回取消函数。同一函数重复注册会被去重，不会重复调用。`closeUI` 会清空全部订阅，之后注册的 handler 不再生效。

### 7.12 Hello

每条 `/bridge/ui` 连接建立后，Host 先发送一条 hello 帧。

```json
{
  "version": 2,
  "id": "ui-push-1",
  "type": "hello"
}
```

第一条 hello 表示首次连接。之后的 hello 表示断线重连，SDK 触发 `onResync`。

### 7.13 多连接

Host 允许多个页面同时连接 `/bridge/ui`。每个连接有独立队列，State、Command 和 Diagnostic 广播给所有连接。新连接不会关闭旧连接，旧页面不会冻结。

## 8. 顺序与恢复

UI Push 使用每个活动连接的有界 FIFO 队列，容量是 64。Publisher 入队后立即返回，不等待浏览器回执。

同一 Mount 的推送保持入队顺序。两个 HTTP 流之间不保证全局顺序。State 使用插件定义的 `revision` 丢弃旧值。插件必须保证 revision 跨进程重启仍递增，否则重连后的新状态会被当成旧值丢弃。Command 需要顺序时，由插件 payload 中的序号处理。

某个连接离线或队列已满时，Host 标记并关闭该连接，其他连接不受影响。
SDK 重连后收到 hello 并触发 resync。Host 不会缓存业务 State，也不会重放 Command。
当没有任何连接接受推送时，`host.ui.state` / `host.ui.command` 返回 `ui_not_mounted`，由插件决定重试还是降级。
Diagnostic 快照单独保存在内存中，上限 256 条，超出后丢弃最旧的。

Host 向所有活动 UI 连接广播推送。连接建立和关闭互不影响。浏览器关闭和重启不影响后端 Call。

刷新或重启后不重放事件。Component 重新调用插件读取方法，Plugin Runtime 重建 State。Shell 重新读取 Bootstrap，恢复当前 Diagnostic 快照。

断线重连后，SDK 触发 resync。Shell 重新读取 Bootstrap，组件通过 `resync()` 或重新挂载重新读取状态。Host 不重放断线期间的 Push。

悬浮窗关闭只影响当前浏览器显示。Host 不删除对应诊断。

### 8.1 重连退避

SDK 在 UI 流断开后自动重连。等待时间按指数增长，上限是 `maxReconnectDelayMs`：

```text
1s → 2s → 4s → 8s → 10s → 10s → ...
```

收到 hello 时退避计数清零。重连没有次数上限，也不做随机抖动。`closeUI()` 是唯一的停止方式。

### 8.2 Diagnostic 合并

Shell 把服务端 Diagnostic 和浏览器本地 Diagnostic 放在一起显示。

合并键是 `plugin`、`path`、`code` 三者的组合。相同键的条目合并为一条，计数加一，显示时在消息后附上次数。

Diagnostic 面板只能整体关闭，没有逐条清除按钮。服务端 Diagnostic 在下一次 bootstrap 时被整体替换，浏览器本地 Diagnostic 累积到页面结束。

## 9. Diagnostic 悬浮窗

Host Shell 拥有 Diagnostic 悬浮窗。它与插件 Component 分离。

悬浮窗遵循以下规则：

- 使用非模态浮层；
- 不显示遮罩；
- 不抢占键盘焦点；
- 不拦截其他组件的鼠标事件；
- 可手动关闭；
- 多条诊断使用可折叠堆栈；
- 相同 `plugin + path + code` 自动合并；
- 不展示 Secret 明文。

命令行窗口始终显示完整诊断。悬浮窗只负责提醒，不承担错误恢复操作。

## 10. Settings Renderer

插件中心提供 `Settings` 页签。页面读取 Manifest 中的 `settings.schema`，并用现有 Browser Call 执行 `settings.get` 和 `settings.save`。

插件未处于 `active` 时，页面不发 `settings.get`，按状态显示未启动、加载中或启动失败提示，并禁用保存。插件启动落定后页面自动刷新快照并加载设置。

页面支持 `string`、`secret`、`number`、`boolean`、`select` 和 `list` 六种控件。`required` 只显示必填提示，不阻止提交。每个插件独立读取和保存。

Secret 输入不回显明文。已有 Secret 显示“已配置，留空保持不变”，空输入是否覆盖由插件定义。保存失败时保留当前输入。

### 10.1 values 包络

读取时调用 `settings.get`，请求 payload 是空对象，响应取 `values` 字段。`values` 不是对象时按空对象处理。

保存时调用 `settings.save`，请求 payload 是 `{values: {...}}`。

各字段的收集规则：

| 字段类型 | 收集规则 |
|---|---|
| `string`、`select` | 非 `null` 时按字符串收集，空串照常提交 |
| `secret` | 只在非空时收集。空串不出现在 payload 里 |
| `number` | 非空且是有限数时按数字收集 |
| `boolean` | 总是收集，按勾选状态取布尔值 |
| `list` | 总是收集为数组。整行无值的行被丢弃 |

Secret 为空时不出现在 payload 中，插件据此保持原有密钥不变。

### 10.2 联动选择

`options_from` 的选项来自被引用 `list` 字段在页面上的当前行，不是已保存的值。

`option_value` 默认 `id`，`option_label` 默认 `label`。取值字段为空白的选项会被过滤掉。

列表行增删或修改时，依赖它的下拉选项立即重建。选项为空时下拉框被禁用。

`item_fields` 的 `select` 类型额外带一个空占位项，保证新加的空行不会被提交。

## 11. 插件中心

Host Shell 的顶栏提供统一的 `插件中心` 入口。中心包含三个页签：

- `插件`：扫描和启停管理；
- `设置`：Settings Renderer；
- `依赖关系`：Host 与插件拓扑图。

插件页按名称展示插件，并列出全部实例。用户可选择一个实例查看 Generation、注册项和失败信息。用户可手动重新扫描，也可启动或停止单个插件。

实例下拉框每项显示实例 id、Generation 和状态。

启动只对硬依赖做提示，不做拦截。

`depends_on` 用来安排启动顺序，不用来许可。调用按 `(capability, method)` 路由，Host 不需要知道目标插件是否活着。缺依赖照样启动，缺失依赖记入诊断。

停止先进入 `stopping`，并等待旧 Call 完成。依赖停止时不级联停止依赖方。停止窗口内的调用返回 `plugin_not_ready`。停止完成后该插件的路由被注销，后续调用返回 `method_not_found`。

依赖图由 Bootstrap 中的插件快照和 `depends_on` 生成。Host 节点使用虚线连接已发现插件，表示装载和路由。

硬依赖实线从依赖方指向被依赖插件。缺失节点显示为 `missing`。实例选择只改变详情展示，不改变依赖图。图下保留同方向的文本关系列表。

点击插件节点后，右侧显示 Capability、硬依赖、被依赖方和 UI Mount。节点高亮当前插件及其一跳关系。

管理接口返回最新 Bootstrap。成功后 Shell 更新插件快照并按新列表对齐 Mount：新增的 Mount 被装载，Bootstrap 中已消失的 Mount 被卸载。面板关闭只影响当前浏览器显示，不改变插件状态。

启动插件后，Shell 会轮询 Bootstrap 直到该插件不再是 `loading`，最长等待 5 秒，再按新列表对齐 Mount。

## 12. 安全约束

- 默认只监听 `127.0.0.1:2333`；
- 所有 `POST` 端点校验 `Origin` 同源，跨源请求返回 403；不带 `Origin` 的请求放行；
- UI 资源目录全部允许，不做 allowlist 或 MIME 白名单；
- 浏览器不能调用 Host 平台管理方法；
- 浏览器不能伪造 `from`；
- Host 校验 mount 属主；
- UI 发布权限由可信 `from` 和 Mount 属主校验确定；
- Secret 不回显；
- v1 插件 JS 是本机可信代码，不提供沙箱隔离。

## 13. 实现状态

当前实现包括：

- Manifest、启动扫描、运行时手动扫描；
- Mount Registry、Host Shell、五 Slot、Component Loader；
- Plugin 状态快照、插件中心、Settings Renderer、依赖关系图、手动启停；
- Bootstrap、插件 UI 资源服务、Browser Call、UI Push 和 Diagnostic Push。

当前代码缺少：

- 文件监听自动加载；
- 自动重启；
- 启停状态持久化。

## 14. 长会话性能

UI State 采用窗口快照加增量 Command。设计、实测和边界见 [WebUI 长会话性能](../design/webui-long-session-performance.md)。
