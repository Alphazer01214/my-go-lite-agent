# 04 — 多文件 ui 资产契约 + uidemo 迁移

**What to build:** 插件 `ui/` 从单 ES Module 扩为多文件资产包：main.js 之外允许 `<template>` HTML 片段与 CSS 文件，组件经 fetch 注入模板、经 adoptedStyleSheets 装载样式；Manifest 校验扩为「UI Entry 与声明的资产文件存在」；Web Medium 静态服务按目录服务（能力已在，补校验联动）。uidemo 迁移为 html/css/js 三文件形态，作为插件作者范本；README 插件 UI 章节同步。

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] uidemo 以多文件资产渲染，行为与迁移前一致
- [ ] Manifest 校验：声明的资产缺失 → Discovery 拒绝；仅 main.js 的单文件插件仍合法
- [ ] Design Token 联动与 `<插件名>-` 前缀校验不回归
- [ ] README 插件 UI 章节反映新契约
