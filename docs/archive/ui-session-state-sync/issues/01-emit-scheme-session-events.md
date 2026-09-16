# 01 — scheme/session 变更事件

**What to build:** scheme set 后 emit `__scheme`；组件订阅 `__session`/`__scheme` 立即 refresh；plugins 图 cache 在相关事件后失效。

**Status:** resolved

- [x] __scheme emit
- [x] agent-status 即时 refresh
- [x] agent-mode-panel 即时 refresh
- [x] plugins-panel cache invalidate

## Answer

refactor-host-boundary Z1 对账确认：本票内容已在既有实现中落地（见上方勾选项
与对应 ADR/测试），状态由 ready-for-agent 滞后修正为 resolved，非本轮新开发。
