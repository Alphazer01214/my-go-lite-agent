# 03 — UI-only 插件合法化

**What to build:** 纯视图插件成为合法形态：Manifest 带 `ui` 块时 `entry` 可省。Discovery 只见其声明；Assembly 可挂载；Host 不为其启动进程、不占 capability 注册表；Web Medium 经 /api/plugins 暴露其 UI Entry 并服务其资产；CLI Medium 无视之。校验规则改为「entry 与 ui 至少其一」。进程为 Frame 而存在——纯视图插件无 Frame 可说。

**Blocked by:** —

**Status:** ready-for-agent

- [ ] 无 exe 的 fixture 插件通过 Discovery / Assembly 并被挂载
- [ ] 浏览器渲染其 Panel Component；host 无对应子进程
- [ ] 既有插件（exe+ui、仅 exe）不受影响；缺 entry 且缺 ui 仍被拒绝
- [ ] CLI Medium 下该插件不产生任何呈现
