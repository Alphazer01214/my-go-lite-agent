# 05 — Multi-session Session 插件

**What to build:** Session 插件支持按 sessionId 创建/打开多个 Session；Host 侧 Session 代理方法带 sessionId（默认可空=当前/唯一会话，保持兼容）。

**Blocked by:** 02 — Turn/Step 边界事件

**Status:** resolved

- [x] `session` Capability 增加 create（append/query/derive 带 sessionId）
- [x] 兼容：未指定 sessionId 时行为与现单会话一致（主缝测试不红）
- [x] Session 元数据：createdAt、可选 parentSession / origin / delegationDepth
- [x] Host `AppendSessionFacts` / `DeriveMessages` / `AgentRequest` / `QuerySessionFacts` 指定 sessionId
- [x] 主缝测试：两个 sessionId 互不串 derive；单会话旧路径回归通过

## Answer

Session 插件改为 `registry`（map[sessionId]*store + meta）。新方法 `session.create`（idempotent，可带 parentSession/origin/delegationDepth）；append/query/derive 接受可选 `sessionId`，空值映射 `default`。Host 四个 Session 代理方法增加 sessionID 参数（空=default）；`agent.request` 载荷可带 sessionId。主缝测试 `plugins/sessionprobe` 在同一 Host 进程内 create/append/derive 双 Session，`TestMultiSessionIsolation` 断言互不泄漏；`TestDefaultSessionBackwardCompatible` 覆盖旧路径。

## Comments

- 存储仍在插件进程内存；持久化不是本票范围。
- 一个 Agent 实例仍只绑一个 Session；多 Session 服务多 Agent 实例。
- Subagent（票 06）依赖本票。
- CLI 暂无 `-session-id`；多会话经 fixture/星型调用验证。
