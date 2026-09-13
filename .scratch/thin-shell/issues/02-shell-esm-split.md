# 02 — Shell 前端模块化拆分（零构建）

**What to build:** Web Medium 的内联单体页面脚本拆为多文件原生 ES modules：槽位布局、Panel 装载器、聊天面渲染、trace 面渲染、SDK 桥各自成模块；静态服务与 go:embed 方式不变，行为零变化。本票是后续「内容搬进插件」各票的 prefactor——先让改动容易，再实施改动。

**Blocked by:** —

**Status:** ready-for-agent

- [ ] 静态资产为多文件 ES modules，不引入任何构建工具链
- [ ] / 与 /trace 行为与拆分前一致（聊天、trace、插件面板、会话栏）
- [ ] KaTeX 等 CDN 依赖不受影响；Design Token 契约不变
