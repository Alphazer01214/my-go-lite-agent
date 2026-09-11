# 06 — Session 插件与日志不变量

**What to build:** Session 作为插件提供仅追加日志的 append/query/derive；Host 在模型请求前强制「Model Context 必须能从 Session Log 重建」，否则拒绝该次请求。

**Blocked by:** 04 — Capability 星型路由

**Status:** ready-for-agent

- [ ] Session 插件声明并提供 append/query/derive（或等价最小集）Capability
- [ ] 日志仅追加；历史/消息由日志派生，不另存一份可变对话数组作为真源
- [ ] Host 在 `agent/request`（或等价模型请求路径）前校验可重建性，失败则拒绝并给出可诊断错误
- [ ] 主缝测试：合法追加后可派生；构造「未入日志却进入模型请求」路径必须被 Host 拒绝
- [ ] Session 可替换存储实现，但不变量不依赖具体 Loop 实现
