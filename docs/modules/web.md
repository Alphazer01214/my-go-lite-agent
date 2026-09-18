# web — Web Medium（HTTP 面 + Shell + 插件 UI）

规范名词：[Web Medium](../../CONTEXT.md)、[Shell](../../CONTEXT.md)、[Panel Component](../../CONTEXT.md)、[PanelOp](../../CONTEXT.md)、[UI Action](../../CONTEXT.md)、[UI Entry](../../CONTEXT.md)、[Design Token](../../CONTEXT.md)、[Plugin Graph](../../CONTEXT.md)、[Status Bar](../../CONTEXT.md)、[New-session Face](../../CONTEXT.md)。相关 ADR：0009、0010、0012、0024、0025、0030。

## 职责分层

| 层 | 路径 | 角色 |
|----|------|------|
| HTTP Medium | `web/`（Go） | L0 HTTP 面、SSE 扇出、插件 UI 静态服务；**无**领域编排 |
| Shell | `web/static/` | 最薄骨架：网格、Design Token、槽位宿主、装载器 |
| Platform Module | `sdk/lite-agent.js` → `/sdk/lite-agent.js` | 作者 SDK（见 [sdk](sdk.md)） |

内容面（Session View、rail、trace、状态芯片）全部是**插件 Panel Component**，不是 Shell 自有实现。

## Go 侧文件

| 文件 | 内容 |
|------|------|
| `server.go` | 路由、SSE、Call 转发、Plugin Graph |
| `embed.go` | `go:embed` shell.html / sdk.js / static/app |
| `gen_sdk.go` | `//go:generate`：复制 `sdk/lite-agent.js` → `web/static/sdk.js` |

## HTTP 路由（L0）

| 路由 | 作用 |
|------|------|
| `GET /` | Shell HTML |
| `GET /events` | SSE；`?replay=1` 环缓冲（默认 500）；keepalive 15s |
| `POST /api/call` | 通用转发 `{to,cap,method,payload}`；hostFaces 走 `CallByFace`，否则 `CallByPlugin` |
| `POST /api/ui-action` | UI Action → `CallByFace(..., "ui", "action")` |
| `POST /api/command` | Slash → `webCommandPlane` |
| `POST /api/tool-approval` | 浏览器裁决（20s 失败关闭 → deny） |
| `GET /api/layout` | 合并后 Layout |
| `GET /api/plugins` | Discovery 目录 + Plugin Graph |
| `GET /plugin-ui/<name>/<rel>` | 插件 UI Entry 静态资源（路径清洗） |
| `GET /sdk/lite-agent.js` | 嵌入的 Platform Module |
| `GET /app/*` | Shell ES 模块，`Cache-Control: no-cache` |

**已知残留**：`/api/call` 对 `session.create` 合并默认 Workspace、对 `agent.loop.turn` 包 status——ADR-0030 收敛中，勿扩大。

**SSE**：`serve.Subscribe` → `event: <topic>\ndata: {topic,data}\n\n`。Topic：`presentation|status|stream|panel|evt|tool_approval`。慢消费者丢 `stream`。挂载时 PanelOps 从 `srv.Panels()` 种子回放。

## Shell（`web/static/`）

| 文件 | 角色 |
|------|------|
| `shell.html` | 三栏网格 + Status Bar + 模块引导 |
| `app/main.js` | 启动：loader、slot→DOM、`/api/layout`、loadPluginUIs、SSE、Plugins/Settings |
| `app/loader.js` | 静态 mount + 运行时 PanelOp set/clear；props 升级竞态 |
| `app/events.js` | 唯一 EventSource；扇出 LiteAgent / panel / status |
| `app/state.js` | `currentSessionId`；静默启动停在 New-session Face |
| `app/settings.js` | 设置浮层：config.schema/get/set；优先 `<name>-settings` |
| `app/plugins-panel.js` | Plugin Graph 模态（SVG 分层） |

### Design Token

`:root` 上 `--la-*`（bg/panel/ink/accent/line/…）。跨 Shadow DOM；组件主题契约唯一来源。

### 插件 UI 装载

1. `/api/plugins` 过滤 `state===mounted` 且有 `ui.entry`
2. 并行 `import(entry?plugin=&v=)`（版本 bust 缓存）
3. 按插件名稳定序应用静态 mount
4. 运行时 PanelOp：创建自定义元素、赋 props、挂 slot

### Wire 形态

- **UI Entry**：Manifest `ui.entry` ES Module，注册 `<plugin>-*`
- **静态 mount**：`/api/plugins` 的 `ui.mounts`
- **PanelOp**：插件 Frame → Host 校验 → SSE `panel` → loader
- **UI Action**：组件 → `LiteAgent.emitUIAction` → `POST /api/ui-action` → 插件 `ui.action`

## 测试

| 测试 | 覆盖 |
|------|------|
| `server_test.go` | Shell/SDK、路径穿越、Plugin Graph 全目录、replay、approval、Workspace 合并 |
| `call_plugin_test.go` | 进程边界 CallByFace config |
| `sdk_sync_test.go` | 嵌入 SDK == 源文件 |
| `cmd/.../web_test.go` | 端到端 turn + SSE |

## 开发约定

1. Shell 不拼聊天流状态机；流式归插件 Panel。
2. 禁止领域 HTTP 面（`/api/session/*`、`/api/turn/*` 已删）。
3. 禁止 Medium 出现插件目录名字面量做调度。
4. 改 `sdk/lite-agent.js` 后必须 `go generate` 同步 `web/static/sdk.js`（sdk_sync_test 会拦）。
5. 破坏 Panel/UI 契约 → 对应 ADR + protocol 升版。
