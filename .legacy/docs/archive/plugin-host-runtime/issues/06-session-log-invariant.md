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

新增 `plugins/session`（基于 `pluginsdk` 的内存 append-only 日志，提供 `session.append/query/derive`）。Host `serve` 增加 `AgentRequest`/`DeriveMessages`/`AppendSessionFacts`/`QuerySessionFacts`：`agent.request` 在调用前用 `session.derive` 重建 Model Context；claimed 非空且与派生结果不一致时返回 `session_invariant_violation`。Host 仅拦截 `agent.request`（其他 `agent.*` 可走插件路由，保留 ADR-0003 可替换性）。CLI：`-session-append` / `-session-query` / `-session-derive` / `-agent-request`，可与 `-invoke` 组合。主缝测试覆盖：追加派生、query、CLI 合法重建、CLI 未入日志拒绝、空日志走私拒绝、空 claimed 重建、插件经星型走私拒绝、插件经星型重建成功。

## Comments

- 存储在插件进程内；换存储实现只需换 Session 插件二进制，Host 不变量逻辑不变。
- 空 claimed（`[]`）语义为「从日志重建」；非空 claimed 必须与 derive 精确一致（v1 不做压缩投影）。
- 默认 Loop（ticket 07）应经 `AgentRequest` 拿派生消息，不得自备可变对话数组。
- Code review 修复：补 query 主缝；`agentprobe` fixture 覆盖插件侧拒绝路径；收紧拦截到 `agent.request`；`Rebuilt` 仅在空 claimed 时为 true。
- `derive` 当前只投影 `type=="message"`；ticket 10 的 additionalContexts 入日志时须使用该 type 或扩展投影。
- Host 侧 `AppendSessionFacts` 仍用 `map[string]any` 透传 fact；若后续要强类型可抽 `serve.Fact`。

