# 06 — 清理与文档收尾

**What to build:** 全仓清扫旧形态残留（`data-la-`、`EmitPanel` HTML 用法、ui/index.html 引用、append/html PanelOp 形状）；README Web Medium 章节改写为组件契约（manifest 形状、PanelOp、SDK、tokens、命名约定、reload 语义）；ADR-0009/0010 与 CONTEXT.md 交叉引用核对；`dist/` 经构建重新生成。

**Blocked by:** 05

**Status:** resolved

- [x] 残留清扫（grep `data-la-` / `EmitPanel` / `index.html` / `"html"`）
- [x] README 组件契约章节
- [x] ADR / CONTEXT 交叉引用核对
- [x] 手动金路径全量走查记录

## Answer

清扫结果：`sdk/lite-agent.js`（可拷贝版）与 `web/static/sdk.js` 同步为新契约；`web/server.go` `/plugin-ui/` 裸路径默认 `index.html`→`main.js`；`pluginsdk.EmitPanel` 注释改 ADR-0010 措辞；uidemo `ui/index.html` 删除。README「Web Medium」段落重写为 Panel Component 契约（manifest 形状、组件命名 `<插件名>-*`、Shadow DOM + `--la-*` tokens、PanelOp set|clear、LiteAgent API、/refresh 整页刷新、uidemo 参考实现指引）。ADR-0009 标注 partially superseded、ADR-0010 落档、CONTEXT.md 增 Panel Component / UI Entry / PanelOp / Design Token 词条并收敛 Panel 词条，三者互引一致。`scripts/build.ps1` uidemo 清单改为拷贝仓库版本；dist/ 全量重建（uidemo: plugin.json + ui/main.js + exe）。残留 grep 仅剩历史记录（.scratch 旧 spec、ADR 正文、词条 Avoid 注记、负例测试）——均为有意保留。

## Comments

- 手动金路径浏览器侧走查记录见 05 票。
