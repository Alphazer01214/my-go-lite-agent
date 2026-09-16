# 01 — scheme/session 变更事件

**What to build:** scheme set 后 emit `__scheme`；组件订阅 `__session`/`__scheme` 立即 refresh；plugins 图 cache 在相关事件后失效。

**Status:** ready-for-agent

- [x] __scheme emit
- [x] agent-status 即时 refresh
- [x] agent-mode-panel 即时 refresh
- [x] plugins-panel cache invalidate
