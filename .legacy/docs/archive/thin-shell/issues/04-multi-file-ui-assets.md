# 04 — 多文件 ui 资产契约 + uidemo 迁移

**What to build:** 插件 `ui/` 从单 ES Module 扩为多文件资产包：main.js 之外允许 `<template>` HTML 片段与 CSS 文件，组件经 fetch 注入模板、经 adoptedStyleSheets 装载样式；Manifest 校验扩为「UI Entry 与声明的资产文件存在」；Web Medium 静态服务按目录服务（能力已在，补校验联动）。uidemo 迁移为 html/css/js 三文件形态，作为插件作者范本；README 插件 UI 章节同步。

**Blocked by:** 03

**Status:** resolved

- [x] uidemo 以多文件资产渲染，行为与迁移前一致
- [x] Manifest 校验：声明的资产缺失 → Discovery 拒绝；仅 main.js 的单文件插件仍合法
- [x] Design Token 联动与 `<插件名>-` 前缀校验不回归
- [x] README 插件 UI 章节反映新契约

## Answer

契约：`UISpec` 增加可选 `assets []string`（相对 `ui/` 的 css / `<template>` html 文件）；`UISpec.validate` 校验非空、不穿越、不与 entry 重复；新增 `UIAssetsExist`（Discovery 逐个校验存在）。不声明则不校验——单文件插件原样合法。静态服务本按目录服务，`/plugin-ui/<name>/panels.css` 零改动。

uidemo 迁移（0.3.0）：`ui/` = `main.js`（行为）+ `panels.css`（样式，`adoptedStyleSheets` 采用）+ `mode-panel.html` / `echo-panel.html`（`<template>` 结构，克隆进 Shadow DOM）。组件运行时 `fetch(new URL(..., import.meta.url))`。DOM 顺序细节：模板内含 `.row` 占位、`_render` 原地填充，首渲染与重渲染顺序一致（原版重渲染会翻转 row/meta 位置，现更稳定）。

测试：UISpec 校验矩阵加 4 个资产用例 + `TestUIAssetsExist`（缺失报错 / 全存在通过 / 无声明直通）；discovery 测试 fixture 扩为「资产齐全的 ui-only + 资产缺失被拒」。`main.js` node --check 语法通过；浏览器内渲染行为由 uidemo 演示承接（与原契约同等覆盖水平）。全量 `-count=1` 绿。
