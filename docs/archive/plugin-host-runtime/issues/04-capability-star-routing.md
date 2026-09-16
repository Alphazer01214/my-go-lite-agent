# 04 — Capability 星型路由

**What to build:** 消费方插件经 Host 调用 `cap.method`；Host 依据提供方清单的 `provides` 选路，转发 Frame 并回传结果。插件进程之间不建立直连。

**Blocked by:** 03 — Assembly 挂载

**Status:** resolved

- [x] Host 维护 Capability → 提供方插件 的注册表（来自已挂载插件的 provides）
- [x] 消费方发出的 `req` 被路由到正确提供方，`res`/`evt` 回到原 `id` 调用方
- [x] 未知 Capability 返回结构化错误，而不是静默挂起
- [x] 主缝测试：两个 fixture 插件（provider + consumer），consumer 只连 Host，断言经星型完成调用
- [x] 不引入插件间直连通道

## Answer

`serve.Server`：长驻插件进程 + provides 注册表 + 转发 id（`fwd-N`）回程路由。consumer fixture 只通过 stdout 向 Host 发 `cap` 请求。Host CLI：`-invoke <plugin> [-call-cap name]`。集成测试覆盖 echo 星型回环与 `capability_unavailable` 结构化错误。

## Comments

- dropPlugin 按 wait.target 失败进行中调用；调用方死亡时丢弃其 pending。
