# WebUI — Host 内核的插件 UI 注册与展示面

**状态**：设计稿（继续 [ui-medium.md](future/ui-medium.md) / manifest `ui` 的落地设计）。
**定位**：WebUI 是 **Host 的一个内核模块**，不是第二套路由、不是业务面。它只做三件事：**注册**插件贡献的 UI 组件、**拼版**到槽位、**桥接**浏览器与 Frame 协议。

名词见 [CONTEXT.md](../CONTEXT.md)；线契约见 [protocol.md](protocol.md)；清单字段见 [manifest.md](manifest.md)；清单类型见 `internal/plugin/manifest.go`。

本文 **§5 注册逻辑** 已与现有代码逐项对齐；**§6 渲染逻辑** 为 Loader 契约。

---

## 1. 定位与边界

### 1.1 一句话

> WebUI = Host 内的 **组件运行时（Shell）+ 注册表 + 协议桥**。  
> 插件以前端 **Component** 注入；Host 像 Vue 的 `app` 一样完成注册与挂载，**不解释业务内容**。

### 1.2 做 / 不做

| 做（WebUI 职责） | 不做（保持 L0 / 薄） |
|------------------|----------------------|
| 读取 `manifest.ui`，登记组件与挂载 | 解析 / 改写业务 payload |
| 提供 Shell 布局与槽位宿主 | 内建 chat / session / agent 等领域面板 |
| 托管插件 `ui/` 静态资源 | 按插件名做业务寻址（无 `to`） |
| 浏览器 ↔ Host 的 Call / Event 桥 | 为每个插件发明 HTTP 业务路由 |
| Design Token、装载生命周期 | 绑定某一家前端框架（Vue/React 运行时） |
| 组件名前缀校验（`<plugin>-*`） | iframe 沙箱（v1 全信任，见 §9） |

### 1.3 与 Host 路由的关系

```text
  浏览器 Component                插件进程
        │                            ▲
        │ LiteAgent.call             │ Frame
        ▼                            │
  ┌──────────── WebUI Bridge ────────┴───┐
  │  from=web 注入 · (capability,method) │
  │  转发进 Host 路由表（与插件 Call 同路） │
  └──────────────────────────────────────┘
```

- 浏览器发起的交互 = 普通 **Request**：`capability` + `method` + `payload`，Host 注入 `from="web"` 后查 **同一张路由表**。
- **禁止** `/api/session/*`、`/api/agent/*` 这类领域 HTTP 面；HTTP 只承载静态资源与桥。
- WebUI 对 Host 路由是 **消费者**，不是旁路。

**命名对齐**：Host 已有 `registerProvides`（Capability 观测表）。UI 注册表是另一张表，API 叫 `webui.Registry.Register / Unregister / Snapshot`，Host 挂接点调用 `registerUI / unregisterUI`，**不得**与 `registerProvides` 混用。

---

## 2. 总览（Vue 对照）

| Vue 概念 | WebUI 对应 | 载体 |
|----------|------------|------|
| `createApp` / 根实例 | Shell | `webui/static/shell.html` |
| `app.component()` 注册 | UI Entry `customElements.define` | `plugins/<name>/ui/main.js` |
| `<component :is>` + `v-bind` | 槽位挂载：tag + props | `manifest.ui.mounts` |
| slot / 插槽 | 五块通用区域 | `top\|bottom\|left\|center\|right` |
| props 下行 | 挂载 `props` + `setData` | manifest / PanelOp |
| emits 上行 | 业务 Call + CustomEvent | `LiteAgent.call` |
| 响应式更新 | Event → 组件刷新 | `presentation.*` / 归因 evt |
| `app.use(plugin)` | Discovery + `manifest.ui` | Host WebUI 注册表 |

**不引入 Vue/React 运行时**。组件模型 = 原生 Web Components。

**props 线字段名统一为 `props`**（与 PanelOp、组件 `setData(props)` 一致；原 `WebMount.property` 已改名）。

---

## 3. 分层

```text
┌──────────────────────────────────────────────────────────┐
│ Browser · Shell + Loader + 插件组件（Custom Elements）      │
│   window.LiteAgent                                        │
└──────────────────────────┬───────────────────────────────┘
                           │ HTTP 静态 + WS/SSE 桥
┌──────────────────────────▼───────────────────────────────┐
│ Host · WebUI 模块（L0）                                    │
│  registry   manifest.ui → 规范化注册表 → Snapshot           │
│  static     /plugin-ui/<name>/* · /sdk/lite-agent.js      │
│  bridge     Call 转发 · Event 扇出 · PanelOp 入站校验       │
└──────────────────────────┬───────────────────────────────┘
                           │ Frame（capability, method）
                 ┌─────────┼─────────┐
                 ▼         ▼         ▼
              session    agent     llm   …
```

---

## 4. 组件模型

### 4.1 UI Entry

```js
// plugins/session/ui/main.js
customElements.define('session-rail', SessionRail);
customElements.define('session-chat', SessionChat);
```

| 约定 | 说明 |
|------|------|
| 标签名 | **必须**以插件名 + `-` 为前缀（`host.ComponentTagSeparator`） |
| 定义方式 | `customElements.define`；Loader 用 `whenDefined` |
| 模块格式 | 原生 ESM |
| 副作用 | 只注册组件与样式，顶层不发业务 Call |

### 4.2 组件契约

```js
class SessionRail extends HTMLElement {
  setData(props) { /* merge → render */ }
  connectedCallback() {
    LiteAgent.call('session', 'list', {}).then(list => this.render(list));
  }
}
```

| 方向 | 机制 |
|------|------|
| props ↓ | 挂载 `props`；运行时 `setData` |
| events ↑ | `LiteAgent.call` / `emit` |
| local | `CustomEvent`（bubbles） |
| 样式 | Shadow DOM + `--la-*` |

状态真源在插件，组件是投影。

### 4.3 挂载（mount）

```text
mount = { id, page, slot, component, props }
```

与 `plugin.WebMount` 一一对应（§5.1）。

---

## 5. 核心：注册逻辑（Host）— 与代码对齐

> 职责：把 `manifest.ui` **校验并写入注册表**，吐出稳定 **Snapshot**。  
> 约束：软失败；不解读业务；**只登记已挂载插件的 UI**。

### 5.1 逐项对齐表（设计 ↔ 代码）

| # | 注册项 | 代码真源 | 对齐结论 |
|---|--------|----------|----------|
| 1 | 清单入口类型 | `plugin.Manifest.WebUI *WebUI`（`json:"ui"`） | 输入 = `*plugin.WebUI`，缺省 nil = 无 UI |
| 2 | entry 字段 | `WebUI.Entry` | 相对 **插件包根**；推荐 `ui/main.js`。不再用「相对 ui/ 目录」 |
| 3 | trust | `WebUI.Trust` | 空 → `"full"`；`isolated` 仅保留字面量 |
| 4 | assets | `WebUI.Assets []string` | 包根相对路径；越界/不存在 → 丢该项 |
| 5 | mounts | `WebUI.Mounts []WebMount` | 见 #6–#9 |
| 6 | 挂载 props 线名 | `WebMount.Props` `json:"props"` | **统一 `props`**（对齐 PanelOp 与 `setData`）；弃用 `property` |
| 7 | 挂载 panel id | `WebMount.ID` `json:"id,omitempty"` | 可选；空则默认 `Component`。对齐 `PanelOp.ID`（按 id 替换） |
| 8 | page 默认 | `WebMount.Page` `json:"page,omitempty"` | 空 → `"main"` |
| 9 | slot 词表 | `host.UISlots` = top/bottom/left/center/right | 静态 mount 也只允许 Shell 五槽 **或** 本插件 `pages[].slots` |
| 10 | 标签前缀 | `host.ComponentTagSeparator = "-"` | `component` 必须 `HasPrefix(name+"-")` 且为合法 custom element tag |
| 11 | pages | `WebUI.Pages []WebPage` | 加法页；slug  `[a-z0-9-]+`，不得占用内建 `main` |
| 12 | 页内槽位 | `WebPage.Slots []string` | **纯 id 数组**；已删除 `WebSlot{role,preferred,region}`（ADR-0031 反域名字） |
| 13 | 注册表类型 | （新建）`internal/webui.Registry` | 与清单输入分离：输入可脏，表内必须已规范化 |
| 14 | 路径清洗 | `utils.NormalizePath` + `filepath.Join` + 前缀判断 | `isUnder(dir, path)`；拒绝 `..` / 绝对路径 / 盘符 |
| 15 | 生命周期挂点 | `Host.launch` / `markUnhealthy` / `disabled` | 见 §5.5；**不**挂在 `registerProvides` |
| 16 | 幽灵 UI | Discovery ≠ mounted | 仅 `launch` 成功后 `Register` |
| 17 | 与 provides 表 | `host.registerProvides` | 两表无关；命名 `registerUI`，避免撞名 |
| 18 | 旧字段 `mountedUI` | `Host.mountedUI` | **废弃**，由 `webui.Registry` 取代 |
| 19 | PanelOp 常量 | `host.PanelOpSet/Clear`、`UISlots` | 注册校验与 UI Op 校验共用同一套词表 |
| 20 | pluginsdk.Layout | `pluginsdk.Layout` 空壳 | **不是**注册真源；见 §12 |

### 5.2 数据结构（Go）

```go
// internal/webui/registry.go

// Registry 是 UI 贡献表（与 host.provides 分离）。
type Registry struct {
    mu      sync.RWMutex
    entries map[string]*PluginUI // key = plugin name
}

// PluginUI：单插件已校验、已规范化的 UI 贡献。
type PluginUI struct {
    Name    string   // Identity（观测，不参与路由）
    Version string   // cache bust ?v=
    Dir     string   // 插件包绝对路径（静态资源根）
    Entry   string   // 规范化相对路径，如 "ui/main.js"
    Trust   string   // "full" | "isolated"
    Assets  []string // 规范化相对路径
    Mounts  []Mount
    Pages   []Page
}

// Mount：由 plugin.WebMount 规范化而来。
type Mount struct {
    ID        string          // §5.3
    Page      string          // 默认 "main"
    Slot      string          // 五槽或本包 page slot
    Component string          // <plugin>-*
    Props     json.RawMessage // nil → {}
}

type Page struct {
    Slug, Title, Path string
    Slots             []string // 本页私有槽 id
}

// Snapshot：GET /api/layout 的响应体。
type Snapshot struct {
    Plugins []PluginView
    Pages   []PageView
    Mounts  []MountView
}

type PluginView struct {
    Name, Version, Entry, Trust string
}

type MountView struct {
    ID, Plugin, Page, Slot, Component string
    Props json.RawMessage
}
```

**清单输入 vs 注册表输出**：`plugin.WebUI` 可含非法项；`webui.PluginUI` 只含合法项。Register 负责丢弃并记 Issue。

### 5.3 身份与幂等

| 概念 | 规则 |
|------|------|
| 注册键 | 插件名。重复 `Register` = **整包替换** |
| Mount.ID | `plugin:page:slot:panelID`，其中 `panelID = WebMount.ID \|\| WebMount.Component` |
| 同 ID 重复声明 | **last-wins** |
| 跨插件同 tag | 不可能（前缀强制） |

与 `PanelOp.ID` 对齐：运行时 `set` 按 `(slot, id)` 替换；静态 mount 的 id 默认即 component 名。

### 5.4 校验流水线（Register）

```text
Register(d plugin.Discovery) []Issue     // 软失败，Issue 只记日志

ui := d.Manifest.WebUI
if ui == nil → 空贡献

── 路径 ────────────────────────────────
1. entry = cleanRel(ui.Entry)            // NormalizePath；拒绝 ..、绝对路径、盘符、://
2. entryPath = filepath.Join(d.Dir, entry)
3. !isUnder(d.Dir, entryPath) 或 !fileExists
     → 拒绝整包 UI（entry_issue: entry_out_of_root | entry_missing）
4. assets 同规则；坏项单独丢弃

── 标签 ────────────────────────────────
5. prefix = name + host.ComponentTagSeparator   // "-"
6. mount.Component: 非空 + 合法 tag（小写、含 '-'、ASCII）
   + HasPrefix(prefix) ；否则丢该 mount（tag_prefix | bad_tag）

── 槽位 ────────────────────────────────
7. pageSlots = {s | p in ui.Pages, s in p.Slots}
8. mount.Page 空 → "main"
9. mount.Slot ∈ host.UISlots ∪ pageSlots（且属于该 page）
   否则丢该 mount（bad_slot）
10. page.Slug：^[a-z0-9-]+$ 且 ≠ "main" 等内建名
    非法/冲突 → 丢该 page 及对其 slot 的引用（bad_page_slug | page_conflict）

── 规范化 ──────────────────────────────
11. panelID = mount.ID || mount.Component
12. Mount.ID = name + ":" + page + ":" + slot + ":" + panelID
13. Props nil → {}
14. Trust 空 → "full"
15. entries[name] = 整包覆盖
```

**Issue 码**（日志/诊断，不进 Frame `error_code`）：
`entry_out_of_root`、`entry_missing`、`asset_out_of_root`、`asset_missing`、`tag_prefix`、`bad_tag`、`bad_slot`、`bad_page_slug`、`page_conflict`。

**与 PanelOp 入站校验共用规则**（§6.5）：tag 合法性、`UISlots`、`HasPrefix(from+"-")`。静态 mount 多一条：允许本包 page 私有 slot；PanelOp 仅允许 `host.UISlots`（与 `validatePanelOp` 史实一致）。

### 5.5 生命周期挂点（对齐 Host）

```text
launch(discovery) 成功          → registerUI(discovery)     // Host 内封装
markUnhealthy(name) / 进程退出  → unregisterUI(name)
disable(name)                   → unregisterUI(name)
enable + 重挂                   → registerUI(discovery)
Host 关闭                       → 清空（不必逐个通知）
```

| Host 现有符号 | UI 注册挂接 |
|---------------|-------------|
| `(*Host).launch` 返回 nil 后 | `Register(discovery)` |
| `(*Host).markUnhealthy` | `Unregister(name)` + 广播 PanelOp clear(plugin) |
| `disabled[name]=true` / unmount | `Unregister(name)` |
| `registerProvides` | **不挂**（另一张表） |
| `mountedUI` | 删除/忽略；勿再写入 |

**发现 ≠ 挂载**：`ScanRoot` 只填 `discoveries`；不 Register。幽灵插件不占槽。

### 5.6 Snapshot 合并

```text
Snapshot():
  plugins = entries 按 Name 升序
  pages   = 内建 main + 各 PluginUI.Pages（slug 冲突：后到丢弃 + Issue）
  mounts  = 全部 Mount，排序 (Page, Slot, Name, ID)
```

Loader **只消费 Snapshot**，不读原始 manifest。

### 5.7 注册面不变式

1. 表内任意 `Mount.Component`：`HasPrefix(pluginName + "-")`。
2. 表内 Entry/Assets：`isUnder(pluginDir)`。
3. `Mount.ID` 全局唯一。
4. `Unregister` 后 Snapshot 无该插件任何项。
5. Register/Unregister 不阻塞插件进程（软失败）。
6. 注册表 **不** 存业务 payload 字段。

---

## 6. 核心：渲染逻辑（Shell Loader）

> 把 `Snapshot` 变成 DOM，并处理 PanelOp / props 更新。无领域选择器。

### 6.1 挂载记录

```js
{
  id: "session:main:left:session-rail",
  plugin: "session", page: "main", slot: "left",
  component: "session-rail", panelId: "session-rail",
  props: {}, el: HTMLElement|null,
  state: "pending"|"mounted"|"failed"|"cleared"
}
```

### 6.2 状态机

```text
fetching → importing（并行 import entry）→ mounting（按序 mountOne）→ ready
                │ 单包失败 → 该包 mounts=failed，不阻塞
ready ⇄ PanelOp set/clear / setData(props)
插件卸载 → unmounting（移除该包全部 el）
```

### 6.3 bootstrap

```text
1. snapshot = GET /api/layout
2. 并行 import(`/plugin-ui/${name}/${entry}?v=${version}`)
3. 按 snapshot.mounts 排序键串行 mountOne
4. ready
```

### 6.4 mountOne / applyProps

```text
mountOne(m):
  el = document.createElement(m.component)  // whenDefined 超时 → failed
  applyProps(el, m.props)
  insertSorted(#region-<slot>, el)          // 保 (plugin, id) 序
  state = mounted

applyProps(el, props):
  if el.setData: el.setData(props); return
  for [k,v] of props:
    if primitive: el.setAttribute(k, String(v)); else el[k] = v
```

### 6.5 PanelOp（运行时拼版）

广播 evt：`capability=presentation` `method=presentation` 的 panel 方法（常量 `PresentationMethodPanel`）。

```json
{
  "op": "set",
  "slot": "right",
  "id": "mode",
  "component": "agent-mode-panel",
  "props": { "scheme": "coding" }
}
```

| 字段 | 必填 | 规则 |
|------|------|------|
| `op` | 是 | `PanelOpSet` \| `PanelOpClear` |
| `slot` | 是 | ∈ `host.UISlots`（**不含** page 私有槽） |
| `id` | 是 | 稳定面板 id；set 按 `(slot,id)` 替换 |
| `component` | set 必填 | 合法 tag + `HasPrefix(from + "-")` |
| `props` | 否 | JSON object |

**Host 入站校验**（对齐 legacy `validatePanelOp`）：上述结构规则；**不**解读 props 业务含义。违规 → 不扇出 + 记 `panel_rejected`（`CodePanelRejected`）。

| op | Loader |
|----|--------|
| `set` | upsert `(slot,id)`：有 record → applyProps；无 → mountOne |
| `clear` | 移除 `(slot,id)`；若 payload 另带 `plugin` 无 component → 整包卸载 |

### 6.6 壳级 props

`loader.broadcastProps({currentSessionId})` → 批量 `setData`。业务状态仍走 Call/Event。

### 6.7 卸载

`Unregister` / PanelOp clear(plugin) → 删该包全部 el。重挂 = 新元素（不热替换）。

### 6.8 渲染面不变式

1. `mounted` 的 el 父节点 = 对应 region。
2. 同 `Mount.ID` 至多一个 live 元素。
3. `failed`/`cleared` 不进 DOM（或错误占位）。
4. Loader 不解释 props 业务字段。
5. 单 mount try/catch。

---

## 7. Manifest 契约（`ui`）— 对齐 `plugin.WebUI`

```json
"ui": {
  "entry": "ui/main.js",
  "trust": "full",
  "assets": ["ui/style.css"],
  "mounts": [
    { "page": "main", "slot": "left",   "component": "session-rail", "id": "rail", "props": { "group_by": "workspace" } },
    { "page": "main", "slot": "center", "component": "session-chat" },
    { "page": "main", "slot": "bottom", "component": "session-status" }
  ],
  "pages": []
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `entry` | string | 包根相对路径 |
| `trust` | string | `full` \| `isolated` |
| `assets` | string[] | 包根相对路径 |
| `mounts[].page` | string | 默认 `main` |
| `mounts[].slot` | string | 五槽 \| 本包 page slot |
| `mounts[].component` | string | `<plugin>-*` |
| `mounts[].id` | string | 可选 panel id |
| `mounts[].props` | object | 初始 props |
| `pages[].slug/title/path/slots` | | 加法页；`slots` 为 **string[]** |

---

## 8. 槽位与布局

五槽宿主：`#region-top|bottom|left|center|right`（常量 `SlotTop`…）。同槽多 mount 按 `(plugin, id)` 序。Shell 不画 Chat/Trace。

---

## 9. 信任与隔离

`full` 唯一实现；`isolated` 预留。前缀校验是防误用。主题只经 `--la-*`。

---

## 10. Author SDK

```js
LiteAgent.call(capability, method, payload)
LiteAgent.emit(capability, method, payload)
LiteAgent.on(capability, method, fn)
LiteAgent.onCall(capability, method, fn)
```

只点名 `capability.method`。WS 优先，降级 `POST /api/call` + SSE。

---

## 11. HTTP 路由（仅 L0）

| 路由 | 作用 |
|------|------|
| `GET /` | Shell |
| `GET /sdk/lite-agent.js` | SDK |
| `GET /plugin-ui/<name>/<rel>` | 静态资源（`isUnder` + `?v=`） |
| `GET /api/layout` | Snapshot |
| `GET /api/plugins` | 观测 / Plugin Graph（可含 Issue） |
| `GET /events` \| `WS /bridge` | 事件扇出 + Call |
| `POST /api/call` | Call 降级 |

---

## 12. 代码落点

```text
internal/webui/
  registry.go     # §5 Register/Unregister/Snapshot/校验
  validate.go     # cleanRel · isUnder · ValidComponentTag · ValidMountSlot
  server.go       # HTTP
  bridge.go       # Call + evt + PanelOp 入站校验
  embed.go

internal/plugin/manifest.go   # 清单输入类型（已对齐 §5.1）
internal/host/proc.go         # launch/markUnhealthy → registerUI/unregisterUI
internal/host/constant.go     # UISlots · PanelOp* · ComponentTagSeparator（复用）

webui/static/… · webui/sdk/lite-agent.js
```

`pluginsdk.Layout`：**不是**注册真源，可删或仅测试用。

---

## 13. 与现有边界对照

| 现行 | 本设计 |
|------|--------|
| UI 非目标 | WebUI 作 Host 模块；仍无按名路由 |
| `ui` 预留类型 | §5.1/§7 已与 `plugin.WebUI` 对齐 |
| `property` / PanelOp `props` 分裂 | **统一 `props`** |
| `WebSlot{role,preferred,region}` | 删除；页槽 = `[]string`（ADR-0031） |
| `mountedUI` | 由 `webui.Registry` 取代 |
| 广播 evt 丢弃 | 需订阅扇出（或垫片） |

---

## 14. 端到端示例

1. `launch(session)` 成功 → `Register`：3 mount（left/center/bottom）。
2. Loader import entry → 三元素进槽。
3. 输入 → `LiteAgent.call('loop','turn',…)`；流式归因 evt → `session-chat`。
4. agent 发 PanelOp `set` (slot=right,id=mode) → Loader upsert。
5. session 死亡 → `Unregister` + clear → 移除。
6. Host 只做注册表、静态托管、`(capability,method)` 转发。

---

## 15. 小结

| 核心 | 契约 |
|------|------|
| **注册** | `plugin.WebUI` → §5.4 流水线 → `Registry` → `Snapshot`；整包幂等；软失败；仅已挂载；与 `registerProvides` 分表 |
| **渲染** | Snapshot → import entry → mountOne → ready；PanelOp set/clear 按 `(slot,id)`；props 走 `setData` |

落地顺序：`webui.Registry` + `validate.go` → Loader 静态 mount → 桥 Call → PanelOp → 出厂插件 UI。
