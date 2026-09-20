# 02 — status bar / mode panel 即时 refresh

**What to build:** agent-status、agent-mode-panel 订阅 `__scheme`/`__session`/status，事件到达立即拉 config。

**Status:** resolved

- [x] subscribe __scheme / __session
- [x] status nudge on ensurePlugins turns

## Answer

refactor-host-boundary Z1 对账确认：本票内容已在既有实现中落地（见上方勾选项
与对应 ADR/测试），状态由 ready-for-agent 滞后修正为 resolved，非本轮新开发。
