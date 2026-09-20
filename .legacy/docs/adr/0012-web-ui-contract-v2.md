# Web UI 契约 v2：磁盘 Layout、插件贡献、Assembly 裁决、protocol 3

Web 开箱无法对话（SDK 缺 sendMessage/runCommand、聊天槽高度链断裂）暴露了 ADR-0011 的契约残缺：slot/page 词表钉死在 Host、Assembly 对 UI 无控制权、平台模块非正式、SDK 双拷贝漂移。在「一切皆插件、作者高自由度」前提下重设计 Web UI 契约。

**决策：**

1. **Layout 磁盘化**：唯一真源是磁盘 `layout.json`（随 dist 发行，`-layout` 可覆盖）。缺失则 server 启动失败。embed 的 HTML 仅是 chrome 渲染器，不再是 layout 语义的来源。推翻 ADR-0011「改 layout 需重编」。
2. **基座 + 加法贡献**：项目 layout 定义内建 page/slot（带 role 与可选 preferred component）。插件经 Manifest `ui.slots`/`ui.pages` 只能新增 page、往既有 slot 追加 mount，不能删改内建 slot 语义。
3. **槽位角色**：每个 slot 有 `role`（如 session-view）与可选 `preferred` 组件名。替代视图可竞争同一 role/slot。
4. **Assembly 裁决**：Assembly 可禁用 mount、覆盖 props/slot/page、指定同槽胜者与顺序。未写则用 Manifest 默认。发现 ≠ 挂载；挂载 ≠ 布局。
5. **导航**：Shell 按合并后 layout 的 page 列表自动渲染导航；插件不自建全局导航。
6. **当前会话下沉**：current/select/list/create 归 session Capability；Web Medium 只做广播与默认绑定。
7. **平台模块入作者 SDK**：markdown/fact 投影列为 Web Medium 平台能力（与 LiteAgent 全局同级），文档化；插件可 import 或自带实现。SDK 单文件双出口（`sdk/lite-agent.js` 为真源，server 同一份挂 `/sdk/`）。
8. **protocol 3**：UI 契约可破坏性变更；不留 HTML 注入/旧 mount 兼容。uidemo 与 session 同期迁移。
9. **信任预留**：Manifest/Layout 可带 `trust: "full"|"isolated"`（默认 full）。本轮只实现 full；isolated 仅校验与文档化，iframe 岛后置（延续 ADR-0010）。
10. **离线**：运行时零 CDN（去掉 KaTeX）。测试可用 Playwright（仅 dev/CI，不进 dist）。

**Considered Options：** layout 仅 embed（保重编语义，作者自由弱）；插件可改栅格（Shell 难保证可用性）；Assembly 点名全部 mount（最强装配权但过繁）。
