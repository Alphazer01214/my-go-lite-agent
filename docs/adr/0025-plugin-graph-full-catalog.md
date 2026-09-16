# Plugin Graph 依 Discovery 全量目录绘制

**Status: accepted**

`/api/plugins` 的插件关系图原先只画「已挂载」集合。在 Autostart + dependsOn（ADR-0021）与 Scheme ensurePlugins（ADR-0023）之下，启动时只有核四件挂载，场景工具要等 Agent 选定 Scheme（也就是**第一轮对话**）才被拉起——于是依赖图在启动后看起来是残缺的，要聊一句才「长出来」。图的读者需要的是「这台机器上有哪些插件、谁依赖谁」，这与「此刻挂了谁」是两个问题，原实现把两者混在了一个节点集里。

**决策：** 图的节点集取 Discovery 全量目录（mount plan ∪ plugins 目录扫描），每个插件节点带 `state`：

- `mounted` —— 进程/UI 已在 Host 中；
- `available` —— 已被发现、尚未挂载（将由 dependsOn 闭包或某个 Scheme 的 dependsPlugins 拉起）；
- `degraded` —— 已挂载但 consumes 未满足（ADR-0022，不进 registry）；
- `missing` —— `dependsOn` 引用了 Discovery 未见到的名字。

新增两类边：`depends-on`（插件名硬闭包）与 `scheme`（Agent Scheme → 其 `dependsPlugins` 成员，带 scheme 名）。响应加 `summary`（discovered / mounted / degraded / missing）与 `schemePulls`。

前端不再向图的语义下注：Shell 启动时**预取**一次快照，打开面板即从缓存同步绘制（无 loading 空白），随后后台刷新并在重绘时更新；节点按 state 着色（实线=已挂载、虚线灰=待拉起、红=degraded、琥珀=missing），侧栏卡片刻 state 徽标与 `pulled by scheme`。渲染异常被捕获并在画布内显示，不再留下空白弹窗。

副作用需要显式处理：`/api/plugins` 现在包含未挂载插件，因此 **Shell 的 UI 装载器与 Settings 浮层按 `state === 'mounted'` 过滤**——未挂载插件的 `ui/` 不在 `/plugin-ui/` 服务范围内，其 PanelOp 也不会被路由。

**Considered Options：** 图只画挂载集但在启动时把 scheme 声明的插件一并「预挂载」（推翻了懒挂载的省资源初衷，否决）；图只画目录但不标状态（读者无法区分「在跑」与「能跑」，否决）；前端在打开面板时才请求（多一次加载空窗，且与首轮对话的时序耦合无法消除，否决）。
