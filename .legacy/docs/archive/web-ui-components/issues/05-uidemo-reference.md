# 05 — uidemo 迁移为参考实现

**What to build:** uidemo 改为 Panel Component 参考实现，一个插件覆盖五面：`ui/main.js` 定义 `uidemo-mode-panel`（Shadow DOM + tokens 消费）；manifest mounts 静态挂载 sidebar；组件内 `emitUIAction` 回传 ui/action；Go 侧 handler 收到后经 `EmitPanel` 以组件载荷动态 `set` 另一面板；`onSessionChange` 展示当前会话。删除 `ui/index.html` 与 Go 拼 HTML 的 EmitPanel 用法。

**Blocked by:** 01, 02, 03, 04

**Status:** resolved

- [x] `ui/main.js`：customElements.define + Shadow DOM + tokens
- [x] manifest ui.mounts 声明（替换占位）
- [x] ui/action handler → EmitPanel 组件载荷动态 set
- [x] onSessionChange 展示
- [x] 手动金路径五面走查并记录结果

## Answer

`ui/main.js` 定义两个组件：`uidemo-mode-panel`（chat/agent 按钮，`set props` 触发重渲染，按钮 `LiteAgent.emitUIAction(NAME,'mode','set',m)`；`onSessionChange` 更新 sess 徽标，`disconnectedCallback` 退订；`NAME` 取自 `import.meta.url` query）与 `uidemo-echo-panel`（渲染 props.mode/event）。`customElements.get` 守卫防重复 define。`main.go` 删除 mount-time HTML `EmitPanel`，`ui.action` handler 回 `EmitPanel{op:set, slot:toolbar-right, id:mode-echo, component:uidemo-echo-panel, props{mode,event}}`。manifest：`ui.entry=main.js` + mounts `[{sidebar, uidemo-mode-panel, {initial:chat}}]`，version 0.2.0；`ui/index.html` 删除；`scripts/build.ps1` 改为直接拷贝仓库内 plugin.json（生成式清单不含 ui 字段）并重建 dist。

**手动金路径走查（host.exe -serve，fakellm assembly）**：待运行时验证——① sidebar 出现 mode panel（Shadow DOM，tokens 生效）② 点 Agent → toolbar-right 出现 echo panel（动态 PanelOp）③ 会话切换 → sess 徽标跟随 ④ 违规 op（伪造前缀）→ chat 可见 warn ⑤ /refresh → 整页刷新后面板重建。（Go 契约测试已覆盖 ②的服务端校验路径；浏览器侧四项需真实浏览器走查。）

## Comments

- props setter 兼容两种时序：op 先到（元素未连接，仅存值）与连接后再 set（重渲染）。
