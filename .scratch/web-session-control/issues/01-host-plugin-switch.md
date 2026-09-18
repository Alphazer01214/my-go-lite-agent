# 01 — Host 插件启用开关

Status: resolved

**What to build:** ADR-0032：disabled 集合持久化、`host.setPluginEnabled`、Start/Ensure/ensureAlive/launch 过滤、运行时卸载、Plugin Graph `disabled`、Web Plugins Panel 开关。

**Acceptance:**
- [x] 关闭已挂载插件后进程消失且 ensure 不会拉起
- [x] 开关持久化到 pluginsDir 下 `.plugin-switch.json`
- [x] Graph/UI 显示 disabled
- [x] Host 源无 `plugins/` 目录名字面量

**Answer:** `serve/plugin_switch.go` + `serve/host_call.go`；Web `/api/call` 到 `host.setPluginEnabled`；Plugins Panel On/Off；测试 `serve/plugin_switch_test.go`。
