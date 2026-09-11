# 04 — Capability 星型路由

**What to build:** 消费方插件经 Host 调用 `cap.method`；Host 依据提供方清单的 `provides` 选路，转发 Frame 并回传结果。插件进程之间不建立直连。

**Blocked by:** 03 — Assembly 挂载

**Status:** ready-for-agent

- [ ] Host 维护 Capability → 提供方插件 的注册表（来自已挂载插件的 provides）
- [ ] 消费方发出的 `req` 被路由到正确提供方，`res`/`evt` 回到原 `id` 调用方
- [ ] 未知 Capability 返回结构化错误，而不是静默挂起
- [ ] 主缝测试：两个 fixture 插件（provider + consumer），consumer 只连 Host，断言经星型完成调用
- [ ] 不引入插件间直连通道
