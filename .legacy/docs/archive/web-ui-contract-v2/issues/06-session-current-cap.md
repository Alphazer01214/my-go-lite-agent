# 06 — 当前会话下沉 session Capability

**Status:** resolved

## Answer

session 增 `current`/`select`；`create` 空 id 铸造 `s-<nano>` 并设为 current。web medium 的 currentSession/setCurrentSession 经 `CallByCap` 与 capability 同步。session-rail/view/main 组件改走 `LiteAgent.call`。Host turn running 态仍走 `/api/session`（媒介级，非 Current Session 真源）。
