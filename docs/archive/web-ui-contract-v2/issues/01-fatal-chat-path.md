# 01 — 修致命路径：SDK 方法 + 槽高度链

**Status:** resolved

## Answer

`sdk/lite-agent.js` 实现并导出 `sendMessage`/`runCommand`；`web/static/sdk.js` 与真源同步（`TestSDKCopiesMatch`）。`#chat` 为 flex 列 + overflow hidden；`session-view` `:host` flex 列、`.flow` 内部滚动、composer 钉底。
