# 06 — Session 插件与日志不变量

**What to build:** Session 作为插件提供仅追加日志的 append/query/derive；Host 在模型请求前强制「Model Context 必须能从 Session Log 重建」，否则拒绝该次请求。

**Blocked by:** 12 — Plugin SDK（二次开发接口）

**Status:** resolved

- [x] Session 插件声明并提供 append/query/derive（或等价最小集）Capability
- [x] 日志仅追加；历史/消息由日志派生，不另存一份可变对话数组作为真源
- [x] Host 在 `agent/request`（或等价模型请求路径）前校验可重建性，失败则拒绝并给出可诊断错误
- [x] 主缝测试：合法追加后可派生；构造「未入日志却进入模型请求」路径必须被 Host 拒绝
- [x] Session 可替换存储实现，但不变量不依赖具体 Loop 实现
- [x] Session 插件基于 `pluginsdk` 实现，不手写 Frame 循环

## Answer

新增 `plugins/session`（基于 `pluginsdk` 的内存 append-only 日志，提供 `session.append/query/derive`）。Host `serve` 增加 `AgentRequest`/`DeriveMessages`/`AppendSessionFacts`：`agent.request` 在调用前用 `session.derive` 重建 Model Context；claimed 非空且与派生结果不一致时返回 `session_invariant_violation`。插件发出的 `cap=agent` 也由 Host 拦截，不旁路不变量。CLI：`-session-append` / `-session-derive` / `-agent-request`。主缝测试覆盖追加派生、合法重建、未入日志拒绝、空日志走私拒绝、空 claimed 走重建。

## Comments

- 存储在插件进程内；换存储实现只需换 Session 插件二进制，Host 不变量逻辑不变。
- 空 claimed（`[]`）语义为「从日志重建」；非空 claimed 必须与 derive 精确一致（v1 不做压缩投影）。
- 默认 Loop（ticket 07）应经 `AgentRequest` 拿派生消息，不得自备可变对话数组。
