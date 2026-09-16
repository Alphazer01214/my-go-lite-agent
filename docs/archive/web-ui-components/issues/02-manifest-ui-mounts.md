# 02 — Manifest ui.mounts 与校验

**What to build:** `plugin.UISpec` 改形：`{entry, mounts: [{slot, component, props?}]}`（删除 slots）。Validate 扩展：entry 必须为 `ui/` 下 `.js`；component 名 `<name>-*`；slot ∈ {sidebar, main-overlay, toolbar-right}；mounts 可缺省。`/api/plugins` 载荷重设为 `{name, provides, commands, ui: {entry, mounts}}`（entry 解析为 `/plugin-ui/<name>/…` URL）。

**Blocked by:** —

**Status:** resolved

- [x] UISpec 改形 + Validate 扩展（entry 形状 / 前缀 / slot 白名单）
- [x] discovery 对 ui.entry 的存在性检查（对齐 executable entry 的处理）
- [x] uidemo plugin.json 同步改形 + `ui/main.js` 空实现占位（保 discovery 绿，05 落地真实现）
- [x] web `/api/plugins` 载荷更新 + 测试
- [x] 测试：manifest 校验矩阵（entry 非 .js / 前缀违规 / 非法 slot / mounts 缺省）

## Answer

`plugin.UISpec{Entry, Mounts []UIMount{Slot, Component, Props}}`；`UISpec.validate(pluginName)` 在 `Manifest.Validate` 内调用（entry 经 slash 归一后必须在 `ui/` 内且 `.js`、不得含 `..`/绝对路径；mounts 的 slot 白名单 `plugin.UISlots`、component 须 `^[a-z][a-z0-9-]*$` 且含连字符、前缀必须 `<name>-`、props 必须是 JSON object）。`NormalizedEntry()` 把 `main.js` 归一为 `ui/main.js`（容忍手写 `ui/` 前缀与嵌套、反斜杠）。Discovery 对 `UI != nil` 追加 `UIEntryExists` 存在性检查（目录即 ScanError）。uidemo 清单先以 `"mounts": []` 占位过 discovery，05 落地真 mounts。`/api/plugins` 载荷（`web/server.go handlePlugins`）：`{name, version, provides, commands, ui: {entry: "/plugin-ui/<name>/<entry 去掉 ui/ 前缀>", mounts: [{slot, component, props}]}}`。测试：`plugin/manifest_test.go`（校验矩阵 + NormalizedEntry + UIEntryExists + ValidUISlot）与 `web/server_test.go`（载荷断言）。

## Comments

- slot 白名单单一真源：`plugin.UISlots`，serve 的 PanelOp 校验复用 `plugin.ValidUISlot`。
- 顺手修了 `plugin/manifest.go` 一处 pre-existing vet 错误（whitespace 错误信息缺格式参数）。
