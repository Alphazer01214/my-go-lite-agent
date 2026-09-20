# 06 — 会话栏插件化

**What to build:** session 插件第二挂载 session-rail 组件（sidebar 槽位）：会话列表、新建、切换经现有媒介级接口完成（「当前会话」留媒介层，是否下沉为 Capability 是后置项）；Shell 的内建会话栏**同票移除**。

**Blocked by:** 05

**Status:** resolved

- [x] 会话列表 / 新建 / 切换全部由 session-rail 驱动，行为如旧
- [x] Shell 不再含内建会话栏
- [x] 与 session-trace 同屏共存，互不干扰

## Answer

session 组件面新增第三挂载 `session-rail`（main 页 sidebar 槽）：brand + ＋ 新建按钮 + 会话列表全部进组件 Shadow DOM（样式随迁）。切换机制改为**事件驱动收敛**：rail 完成媒介级调用（POST /api/session/select 或 /api/session/new）后设 `window.__liteSessionId` 并 `LiteAgent.emit('__session', id)`；main.js 订阅 `__session` 统一执行「清 UI → loadHistory → refreshRunState」——原 selectSession/btnNew 的编排逻辑随之删除，所有切换路径（rail、未来组件）共用一条。setSessionId 自身的同值守卫保证 refreshRunState 不触发循环刷新。

rail 列表刷新时机：装载时、`status` 主题的 idle/error、切换/新建后——即原 events.js 里 loadSessions 的所有触发点（该职责从 events.js 移除）。

退役：shell 内建 rail 全链（rail.js 模块、#rail-head/#session-list/.sess-item CSS、btnNew 编排）。Shell 布局中 `#rail` 变成纯槽位宿主。全量 `-count=1` 绿；node --check 过。
