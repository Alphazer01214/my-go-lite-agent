# 05 — trace 视图插件化 + layout 页面机制

**What to build:** layout 定义页面（/ 主页面、/trace 调试页），每页是纯槽位网格；mounts 增加 page 维度（缺省 main，非法值拒绝）。session 插件获得 `ui` 块，首个挂载 session-trace 组件（trace 页），会话事实经 capability 调用（`call('session','query')`）自取并投影。Shell 的内建 trace 页面与 /api/trace 端点**同票退役**，杜绝双重渲染；build.ps1 拷贝 session 插件的 ui 资产。

**Blocked by:** 02, 04

**Status:** ready-for-agent

- [ ] /trace 由 session-trace 组件渲染，事实内容与旧 trace 页一致
- [ ] mounts.page 缺省 main；非法 page 值被拒绝
- [ ] /api/trace 与内建 trace 页面移除，无回归
- [ ] session 插件经 Assembly 挂载即生效其 Web 面，无需改 Host 代码
