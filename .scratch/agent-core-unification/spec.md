# Agent / LLM / Session 统一对齐

Status: ready-for-agent

## Problem Statement

plugin-host-runtime 已落地 Host + 进程外插件 + 默认 Loop + Session 不变量，但 agent 相关领域模型长期两张皮：CONTEXT.md 缺 Agent / Agent Loop / Turn / Step 词条；system-prompt 在 ADR-0003 提了却从未实现；Session Log 无 turn 边界与 request header；subagent 显式 out-of-scope 但词汇与接缝未留。参考 deepseek-harness 的 Agent/Session/Turn/Step/SystemPrompt 模型，需要把词汇、边界与实现缺口一次钉住，避免后续实现继续漂移。

## Solution

领域层：CONTEXT.md 补齐 Agent、Agent Loop、Turn、Step、System Prompt、Request Header、Subagent、Context Manager、Prompt Segment；ADR-0004/0005/0006 定型 Agent=插件、LLM 解析先于可见提交、System Prompt 作 Capability。

实现层按依赖顺序补缺口：Context Manager 插件（system-prompt 组装）→ Session 落 turn/step 边界与 request header → Loop 命名与术语对齐 → multi-session Session 插件 → Subagent 同步 tool result。

术语以仓库 `CONTEXT.md` 为准；架构约束以 `docs/adr/0001–0006` 为准。

## User Stories

1. 作为插件作者，我想用 `system-prompt.registerSegment` 注册有序 Prompt Segment，以便我的能力说明进入 System Prompt 而不改 Host。
2. 作为 agent 使用者，我想 Assembly 里配置基础 Prompt Segment，以便人格/安全规则在启动时确定。
3. 作为 Agent Loop，我想在首个 Step 前调用 `system-prompt.assemble`，把结果 append 成 Session Log 的 system 事实，以便「模型可见即已记录」。
4. 作为会话维护者，我想 Session Log 记录 `turn/start`、`turn/end`、`step/start`、`step/end`，以便审计与 UI 能还原执行层级。
5. 作为会话维护者，我想每次模型调用把 Request Header 快照落入 Session Log，以便重放时知道用的是哪套 provider/model 参数。
6. 作为排障者，我想 `MaxToolRounds` 等命名与 Turn/Step 词汇对齐，以便代码与词汇表不再各说各话。
7. 作为 Session 插件作者，我想支持按 sessionId 创建/打开多个 Session，以便 Host 能同时托管父 Agent 与 Subagent。
8. 作为 agent 使用者，我想父 Agent 经同步 tool call 触发 Subagent（新 Agent 实例 + 新 Session），并以 tool result 拿到子产出，以便受限子任务不污染父历史。
9. 作为 Subagent 触发方，我想在调用里声明工具子集与 System Prompt 片段（未声明则继承全局），以便子任务权限可控。
10. 作为 Subagent 触发方，我想在 tool call 里声明同步或异步模式；v1 先保证同步路径完整，异步留给 Inbox 立项后的后续票。

## Implementation Decisions

- **Agent 是插件**（ADR-0004）：provides `loop`；Host 永久保留 `agent.request` / `agent.inject` 横切面，不因 Agent 升格而下放不变量。
- **一个 Agent 实例 ↔ 一个 Session**；Host 可托管多个 Agent 实例（含 Subagent）。
- **LLM 契约 v1 仍为 `llm.complete`**，但 Loop 顺序固定为：AgentRequest 重建 → 调 llm → 成功才 append（ADR-0005）。
- **Context Manager = 插件**，Capability 名 `system-prompt`（ADR-0006）。方法集 v1：`registerSegment` / `registerContext` / `assemble`。
- **Context Manager 只做组装**：动态 context 简单并入 System Prompt 文本尾部；不做 surface replace / compaction / token meter / `{{var}}` 插值 / scoped shadow。
- **System Prompt 落盘**：Loop 调 assemble 后 append 为 system 事实；`session.derive` 继续负责 history 投影。Context Manager 不包住 derive。
- **Prompt Segment 来源**：Assembly 静态段 + 插件运行时 `registerSegment`；排序 `(order, name)`。
- **Subagent**：新 Agent 实例、新 Session；v1 同步 tool result；工具子集/System Prompt 片段由调用方声明；`parentSession` / `origin` / `delegationDepth` 进 Session 元数据。
- **Session Fact 扩展**：`type` 增加 turn/step 边界与 `request_header`；derive 只投影模型可见消息，边界/header 不进 Model Context。

## Testing Decisions

- 继续唯一主缝：真实 Host 可执行 + fixture 插件进程；不断言 Host 内部函数序。
- Context Manager：fixture 段注册后 assemble 文本可断言；无 system-prompt 提供方时 Loop 行为明确（空 system 或 fail-loud，票内定死）。
- Turn/Step/Header：一轮结束后 `session.query` 能列出边界与 header 事实；derive 结果不含这些 type。
- Subagent：主缝测「父 tool call → 子 Session 独立完整 turn → tool_result 回父」；子日志不混入父 derive。

## Out of Scope

- 历史压缩 / surface replace / token budget / KV-cache 友好 in-history system 替换（需 Session replace 能力，独立 feature）。
- Inbox、followup、steer 唤醒语义。
- 异步 Subagent 完整生命周期与取消传播。
- 真实 LLM provider 的两阶段 `prepareCall` 方法拆分（领域约束已由 ADR-0005 覆盖）。
- 多 Interceptor 组合、细粒度 cancel token（既有留白，不在本 feature）。
- Goal/Ralph 等外层 Round 策略。

## Further Notes

- 规范名词一律以 `CONTEXT.md` 为准；实现若与 ADR-0001–0006 冲突，先改 ADR 再改代码。
- 参考：deepseek-harness 的 Agent.id===Session.id、Turn/Step、systemPrompt 注册表装配、System Prompt 作 surface 节点、Compaction 独立 capability。映射到本仓库进程插件模型，不引入 Cordis。
- 建议实现顺序：01 Context Manager → 02 Turn/Step 日志 → 03 Request Header → 04 Loop 命名对齐 → 05 multi-session → 06 Subagent 同步路径。
- 分工：`plugins/contextmanager`（或 `plugins/systemprompt`）+ `serve` Loop 接线 + `plugins/session` 多会话扩展。
- 后续 tickets 拆到 `.scratch/agent-core-unification/issues/`。
