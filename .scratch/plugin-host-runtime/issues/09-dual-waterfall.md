# 09 — 双层 Waterfall

**What to build:** Host 内建强制 Waterfall 链（至少审计与取消相关最低集）始终生效；外部 Interceptor 插件可挂载，对调用放行、改写或短路，且不能移除内建强制链。

**Blocked by:** 05 — 生命周期与恢复

**Status:** ready-for-agent

- [ ] 内建强制链在每次相关调用上可观察（例如审计记录或测试探针）
- [ ] Interceptor 插件经 Host 挂入 Waterfall，可改写 payload 或返回拒绝
- [ ] 短路时下游不执行，调用方收到与拒绝语义一致的错误/结果
- [ ] 卸载或崩溃 Interceptor 不导致强制链消失
- [ ] 主缝测试：无 Interceptor / 允许 / 改写 / 拒绝 四种路径的外部行为
