# Web UI 组件化（Panel Component）

Status: implemented (issues 01–06 resolved)

## Problem Statement

ADR-0009 的 Panel 注入是 HTML 字符串经 innerHTML 落地：浏览器不执行 innerHTML 插入的 `<script>`，插件作者实际写不了自己的 JS，只能靠 `data-la-*` 让宿主代绑事件；Go 侧 `EmitPanel` 还在诱导在 Go 里拼 HTML。目标：插件作为 component、Shell 作为 container——插件作者自行实现 html/js/css 并注册至宿主，Host 零 Web 渲染编码。

## Solution

### 1. 决策总览（grill 已收敛）

| 决策点 | 结论 |
|---|---|
| 本期范围 | 仅 Panel 组件化；Presentation Card 的 UI 消费另立工作流 |
| 组件技术 | 原生 Web Components（customElements.define），零构建链 |
| 样式隔离 | Shadow DOM (open) + Shell `--la-*` Design Token |
| 信任模型 | 维持完全信任（无 iframe/CSP）；ADR-0010 明示信任升级 |
| 契约演进 | 激进替换，不留 html/append/slots 兼容；protocol 保持 2 |
| 命名治理 | PanelOp component 前缀 Host 强制校验；define 为文档约定 + 原生 upgrade 容忍 |
| reload | 插件变更广播 → Shell 整页刷新 |
| SDK | 最小增量：onSessionChange；删 bind/data-la-* |
| 测试 | 零 JS 自动化测试；Go 契约测试 + uidemo 手动金路径 |

### 2. Manifest 形状

```json
{
  "ui": {
    "entry": "main.js",
    "mounts": [
      { "slot": "sidebar", "component": "uidemo-mode-panel", "props": { "initial": "chat" } }
    ]
  }
}
```

- `entry`：`ui/` 下 ES Module（UI Entry），相对插件目录；Shell 解析为 `/plugin-ui/<name>/<entry>?v=<version>`。
- `mounts` 替换 `slots`；可缺省（只做动态 PanelOp 的插件）。
- 校验（Discovery/Assembly）：entry 为 `ui/` 下 `.js` 且存在；component 名 `<name>-*`；slot ∈ {sidebar, main-overlay, toolbar-right}。

### 3. PanelOp 形状

```json
{ "op": "set", "slot": "sidebar", "id": "mode", "component": "uidemo-mode-panel", "props": { "mode": "agent" } }
{ "op": "clear", "slot": "sidebar", "id": "mode" }
```

- `set` 按 id 重挂载（移除旧元素 → 新建 `<component>` → property assignment 设 `props` → 插入）；`clear` 按 id 移除；`append` 与 `html` 删除。
- serve 转发前校验 op 枚举与 component 前缀（以发出插件名为准），违规拒绝并回错误 Frame。
- upgrade 容忍：元素先于模块加载插入时依赖原生 custom element upgrade，props 不丢。
- 更新语义是「重挂载」：组件不假设跨 set 存活；需持久的状态放插件后端。

### 4. Shell 挂载器与 reload

- 启动：`GET /api/plugins` → 对每个有 ui 的插件 `import(entryURL)` → 按 mounts 落位。
- 运行时：处理 PanelOp set/clear（替换旧 innerHTML applyPanel 路径）。
- 收到插件变更广播 → `location.reload()`。
- 删除 `web.injectManifestUIs`（Go 读 HTML 的路径）。

### 5. Design Tokens 与 SDK

- tokens：`--la-bg / --la-panel / --la-ink / --la-dim / --la-line / --la-accent`；Shell 自身样式同源消费。
- SDK：`onSessionChange(fn)`（订阅即以当前 session id 回调一次）；删 `bind` 与 `data-la-*`；`on / call / emitUIAction / complete` 不变。

## User Stories

1. 作为插件作者，我手写一个 `ui/main.js`（define + Shadow DOM）并在 Manifest 声明 mounts，无需 Node 构建即可交付带交互的业务面板。
2. 作为插件作者，我的组件内部直接 `LiteAgent.emitUIAction` / `call` 回传，并经 `onSessionChange` 响应会话切换。
3. 作为维护者，Go 侧不再出现任何 HTML 字符串；渲染契约全部在 Shell 与组件 JS 中。
4. 作为使用者，`/refresh` 后浏览器整页刷新，面板与插件状态一致。

## Implementation Decisions

- 声明式优先：静态挂载走 Manifest（与 commands 同哲学），运行时走 PanelOp；不做模块运行时自注册挂载（宿主不可静态审查）。
- 激进替换：仓库内 Panel 唯一消费者是 uidemo，无外部作者；留兼容路径只会让两种模型并存。
- Shadow DOM 而非 light DOM：插件 CSS 互染与染 Shell 是实际风险（uidemo 内联 style 即规避痕迹）。
- 零 JS 测试设施：与「前端零 Node」一致；Shell JS 保持极简以压低无测试风险。
- 范围切割：Card 的 UI 消费复用本期组件基座，但动的是聊天面渲染规则，另立工作流。

## Testing Decisions

- Go 契约测试：PanelOp 校验矩阵（op 枚举、前缀违规拒绝、slot 白名单）、Manifest ui 校验矩阵、`/api/plugins` 载荷、`/plugin-ui/` 服务 `.js` 与 traversal 回归、mount 时重放路径。
- 手动金路径（uidemo，五面）：Shadow DOM + tokens、props 传递、UI Action 回传、PanelOp set/clear 动态更新、会话切换响应；upgrade 顺序（op 先到）一并在金路径覆盖。
- CLI 回归不破（CLI 忽略 panel 的行为不变）。

## Out of Scope

- Presentation Card 的 UI 消费（聊天流内卡片渲染）。
- iframe/CSP、不可信插件沙箱、热替换。
- npm/前端构建链、移动端、新增 slot。

## Issue 分解（建议）

| # | 标题 | 依赖 |
|---|---|---|
| 01 | PanelOp 组件载荷 + serve 校验 | — |
| 02 | Manifest ui.mounts + 校验 + uidemo 清单改形 | — |
| 03 | Shell 组件挂载器（import/mounts/PanelOp/reload） | 01, 02 |
| 04 | Design tokens + SDK 增量 | — |
| 05 | uidemo 参考实现迁移（五面金路径） | 01–04 |
| 06 | 清理与文档收尾 | 05 |

## Further Notes

- 术语以 CONTEXT.md（Panel / Panel Component / UI Entry / PanelOp / Design Token）与 ADR-0010 为准；ADR-0009 标注 partially superseded。
- 工作分支：`feat/web-ui-components`。
- `dist/` 为构建产物，经 `scripts/build.ps1` 重新生成，不手改。
