# uidemo

> 状态：示例 / 参考（Web Panel Component 的活文档）；不 autostart，也无 scheme 拉起。

Web Panel Component 的参考实现（ADR-0010 / ADR-0011），一份插件演示完整契约：

1. **静态挂载** —— `plugin.json` 的 `ui.mounts` 把 `uidemo-mode-panel` 放进 sidebar
2. **Shadow DOM + 外部资产** —— 样式/模板来自 `ui/` 下的独立文件，主题走 `--la-*` Design Token
3. **UI Action** —— 按钮经 `LiteAgent.emitUIAction` 回到插件的 `ui.action`
4. **动态 PanelOp** —— 插件回 `EmitPanel`，把 `uidemo-echo-panel` 装进 toolbar-right
5. **会话感知** —— `LiteAgent.onSessionChange` 维持 footer 的当前会话

## Manifest

- `ui.entry: main.js`，`ui.assets: panels.css / mode-panel.html / echo-panel.html`
