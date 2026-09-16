# 01 — PanelOp 组件载荷与 serve 校验

**What to build:** `pluginsdk.PanelOp` 与 `serve.PanelOp` 改形为 `{op: set|clear, slot, id, component, props}`（删除 html 与 append）。serve 转发前校验：op ∈ {set, clear}、component 名以发出插件名 + `-` 为前缀、slot 合法；违规拒绝该 op 并回错误 Frame，不静默丢弃。`EmitPanel` 同步改形。

**Blocked by:** —

**Status:** resolved

- [x] PanelOp 结构改形（pluginsdk / serve 两处一致）
- [x] serve 校验：op 枚举、component 前缀（按发出插件名）、slot 白名单；违规 nack
- [x] mount 时 panel 重放路径（`Panels()` seed）适配
- [x] 契约测试：合法 set/clear 广播；前缀违规拒绝；html/append 形状不再被接受
- [x] CLI 忽略 panel 的回归不破

## Answer

`PanelOp` 两处（`serve/serve.go`、`pluginsdk/presentation.go`）改形为 `{op set|clear, slot, id, component, props}`；`collectEvent` 现在携带 `from`（发出插件名），`dispatchPanel` 经 `validatePanelOp` 校验（op 枚举 / id 必填 / slot 白名单 `plugin.UISlots` / set 必须带合法 tag 且 `<from>-` 前缀 / props 为合法 JSON），违规经 `rejectPanel` 抑制广播。**与票面的偏差**：panel evt Frame 无 id、协议层无回执通道，「回错误 Frame」落地为 `OnStatus` warn + 广播 `status` warn 事件（可观测、不静默）。测试 `serve/panel_test.go`：接受/拒绝矩阵 + 原始 payload（legacy html 形状、坏 JSON props）+ `ValidComponentTag`。

## Comments

- `ValidComponentTag`：小写字母开头、含连字符、`[a-z0-9-]` 字符集。
- `Panels()` replay ring 不变，载荷自动随新形状。
