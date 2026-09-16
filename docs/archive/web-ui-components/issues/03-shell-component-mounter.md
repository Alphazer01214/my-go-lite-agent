# 03 — Shell 组件挂载器

**What to build:** shell.html 增组件挂载器：启动经 `/api/plugins` 对每个有 ui 插件 `import(entryURL)`，按 mounts 在对应槽创建元素并以 property assignment 设 props；处理 PanelOp `set|clear`（set 按 id 重挂载、clear 按 id 移除），替换旧 innerHTML applyPanel；收到插件变更广播 → `location.reload()`。删除 `web.injectManifestUIs` 的 Go 读 HTML 路径。

**Blocked by:** 01, 02

**Status:** resolved

- [x] mounts 静态挂载（import + 建元素 + props + 落位）
- [x] PanelOp set/clear 运行时处理
- [x] 插件变更广播 → 整页刷新
- [x] 删 injectManifestUIs 与 ui/index.html 读取逻辑
- [x] web_test 适配（HTML 注入断言改为挂载器契约断言）

## Answer

`shell.html`：`applyPanel` 重写为组件模型——`set` 按 id 移除旧元素后 `createElement(op.component)` 并 property-assign `props`（op 先到、模块后加载时由原生 custom element upgrade 接住）；`clear` 按 id 移除；槽位映射 sidebar→#rail、main-overlay→#main-overlay、默认 toolbar-right。`loadPluginUIs()` 在启动时 fetch `/api/plugins`，对每个 `ui.entry` 做 `import(url + '?plugin=<name>&v=<version>')`（query 供组件从 `import.meta.url` 取名；version 做 cache-busting），成功后逐 mount 应用（panel id = component tag）；失败打 chat 错误行。`/refresh` 命令成功后 `location.reload()`（移除旧 `rehydrateShell` 路径）。`web/server.go` 删除 `injectManifestUIs`；`/plugin-ui/<name>/` 裸路径默认从 `index.html` 改为 `main.js`。SSE 转发：`applySSE` 把全部 topic 经 `LiteAgent.emit` 扇出给组件。测试 `web/server_test.go` 改为断言 entry module 可服务、`/api/plugins` 契约字段、traversal 回归（删掉 mount-time HTML replay 断言）。

## Comments

- 静态 mount 的 panel id 用 component tag（天然唯一且与动态 op 可互换）。
- 模块 URL 带 `?plugin=` 是组件侧取插件名的唯一可靠途径（`location` 是 Shell 页面，不是模块）。
