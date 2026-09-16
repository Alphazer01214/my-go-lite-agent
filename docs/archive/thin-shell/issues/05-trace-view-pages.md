# 05 — trace 视图插件化 + layout 页面机制

**What to build:** layout 定义页面（/ 主页面、/trace 调试页），每页是纯槽位网格；mounts 增加 page 维度（缺省 main，非法值拒绝）。session 插件获得 `ui` 块，首个挂载 session-trace 组件（trace 页），会话事实经 capability 调用（`call('session','query')`）自取并投影。Shell 的内建 trace 页面与 /api/trace 端点**同票退役**，杜绝双重渲染；build.ps1 拷贝 session 插件的 ui 资产。

**Blocked by:** 02, 04

**Status:** resolved

- [x] /trace 由 session-trace 组件渲染，事实内容与旧 trace 页一致
- [x] mounts.page 缺省 main；非法 page 值被拒绝
- [x] /api/trace 与内建 trace 页面移除，无回归
- [x] session 插件经 Assembly 挂载即生效其 Web 面，无需改 Host 代码

## Answer

页面机制：`UIMount.Page`（缺省 main；slug 语法校验在 Manifest，页面词汇表归 layout——`loader.js` 的 `PAGES=['main','trace']`）。布局两页皆纯槽位网格：`/` 主页面（rail / chat / **slot-trace** / toolbar-right / main-overlay）与 `/trace`（**slot-main**）。

装载器重构：`loader.js` 的 `createLoader(page, panelHost, onError)` 取代 panels.js——静态挂载按 page 过滤，PanelOp 经页面实例的 panelHost 路由（无对应槽位即跳过）；main.js 创建主页面实例并经 `setPageLoader` 提供给 events.js；trace-page.js 创建 /trace 实例。

session 插件 Web 面：随仓 `plugins/session/plugin.json`（build.ps1 按 uidemo 模式拷贝）+ `ui/main.js` 定义 `session-trace`，双挂载（main 页 trace 列、/trace 页 main 槽）。数据走 `call('session','query')` 星型路由，当前会话经 GET /api/session（媒介级状态）；2s 轮询对齐旧页行为 + `onSessionChange` 与 `on('session')` 增量。

退役：`/api/trace` 端点、shell 内建 trace 渲染全链（trace.js 模块、trace-list、btn-dump、.trace-row CSS）。web_test 的持久化断言改走 `/api/call` → session.query（正是组件的数据路径）。槽位词汇：`UISlots` 增 `trace`（主页面中央列）与 `main`（/trace 页单槽）——名称页内命名空间。

偏差：原票面未提主页面中央 trace 列——它同样是 shell 内建内容渲染，本票一并插件化（session-trace 同组件双挂载），否则 08 的「Shell 归零内容」无法达成。全量 `-count=1` 绿。
