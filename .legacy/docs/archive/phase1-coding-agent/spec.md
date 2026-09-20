# Phase 1 Coding Agent（对标 Codex/CC 的默认可跑组合）

Status: ready-for-agent

## Problem Statement

Host + 进程外插件骨架已齐（Session Log、Context Manager、Agent Loop、filetools 五件套、CLI/Web），但相对 Codex CLI / Claude Code 仍缺「日常写代码」表注能力：无 Workspace 根、无 shell、无 Permission/sandbox、无项目上下文文件、Skill 仅目录占位、无 Todo/计划可见性、tools 不可多插件共存。ADR-0014 已明确策略不得恢复 Interceptor；filetools 自注 sandbox TBD。需要在不推翻「一切皆插件 / Host 薄内核 / Session Log 唯一真源」的前提下，补齐 Phase 1 可运行的 coding 组合。

## Solution

以 **Workspace（Session 元数据）** 为统一根，落地：

1. Host：`tools` 多提供方汇聚（ADR-0018）。
2. **permission** 插件：`policy` Capability + 双层配置（ADR-0019）。
3. **shelltools** 插件：Win/mac/Linux 默认解释器可配，无 PTY。
4. **filetools** 强化：认 Workspace、schema `readOnly`、路径越界交 policy。
5. **project-context**：`AGENTS.md`/`CLAUDE.md` → Prompt Segment。
6. **skill-manager**：`.liteagent/skills/` 发现、CM 联动、`$skill` 用户输入触发、`load_skill` 工具。
7. **agent** 强化：`policy.decide` + ask→`agent.request`、Todo 工具、只读并行、`$skill` 展开。
8. **context-manager**：Todo 事实投影进 prepare。
9. 金路径集成测试 + `examples/coding.json`。

MCP、OS 级 sandbox、hooks 脚本、benchmark、并行写命令、PTY：**Phase 2+ backlog**（见文末），本轮不实现、不预开票。

## User Stories

1. 作为使用者，我在 CLI 于项目目录启动后，agent 读写与 shell 都限制在该 Workspace。
2. 作为使用者，我在 Web 新建会话时选择 Workspace，多会话可指向不同目录。
3. 作为使用者，我想在 `.liteagent/permissions.json` 里 deny 某类 shell/write，agent 调用被拒绝且可在 Trace 看到。
4. 作为使用者，我想危险调用在 CLI/Web 被 ask，确认后才执行。
5. 作为使用者，我想 agent 跨 Win/mac/Linux 跑测试命令，解释器默认正确、可配置。
6. 作为使用者，我想工作区 `AGENTS.md` 自动进入 System Prompt。
7. 作为使用者，我输入 `$review` 时，对应 Skill 全文在进入模型前注入。
8. 作为使用者，我想模型也能按需 `load_skill`，与 `$` 触发并存。
9. 作为使用者，我想 agent 维护可见 Todo，计划阶段只靠提示约束、不切换工具门禁。
10. 作为插件作者，我想独立提供 tools 插件而不与 filetools 争唯一属主。
11. 作为维护者，我想金路径测试覆盖：读仓 → 改文件 → 跑命令 → deny/ask。

## Implementation Decisions

- **Workspace**（ADR-0020）：Session 元数据；agent 在 `tools.call` payload 注入 `workspace`；工具插件不得猜 cwd。CLI 默认 cwd；Server 会话级选择。
- **配置双层**：`<exe 旁 config/>` 默认，`<Workspace>/.liteagent/` 覆盖；浅合并字段覆盖，不深合并数组。文件名 `permissions.json`（及后续 `config.json` 类）。
- **Policy**（ADR-0019）：`policy.decide({tool, arguments, workspace}) → {action: allow|ask|deny, reason}`。规则 `deny > ask > allow`。agent 在 callTool 前查询；无 permission 插件时视为 allow（软失败装配，ADR-0017）。
- **ask**：agent → Host `agent.request` → Render Medium；结果与决策事实写入 Session Log。
- **tools 多提供方**（ADR-0018）：list 合并、call 按工具名路由、启动期同名冲突 fail-loud。
- **readOnly**：tools schema 扩展字段，由工具作者声明；agent 只读工具可并行，写/shell 串行。
- **shelltools**：Win 默认 `powershell.exe -NoProfile -Command`；Unix 默认 `/bin/sh -c`；可配 `cmd`/`pwsh`/`bash`。timeout + 输出截断；无 PTY。路径入参 slash，插件内归一。
- **Skill 布局**：`<Workspace>/.liteagent/skills/<name>/SKILL.md`。skill-manager 发现 → CM `registerSkill` 目录 + `load_skill` 工具；agent Turn 开始解析用户输入 `$name` 展开。
- **Todo**：agent 内建工具（非独立插件）；事实落 Session Log；CM `context.prepare` 投影最新列表。
- **Plan Constraint**：仅 Prompt 段 + Todo 可见性；**不做** tools 子集门禁（有意偏离 Codex/CC）。
- **project-context**：读 Workspace 根 `AGENTS.md`，回退/并读 `CLAUDE.md`；注册为 Prompt Segment。
- **官方装配**：`examples/coding.json` = agent + session + llm-openai + context-manager + filetools + shelltools + permission + skill-manager + project-context。

## Testing Decisions

- 主缝不变：真实 Host + 真实插件进程；不断言 Host 内部函数序。
- 必测：
  - Workspace 注入后 filetools 相对路径落在根内；越界 write 被 policy deny。
  - tools 双提供方 list 合并、call 分路由；同名工具启动失败。
  - shell 三平台各至少一条 happy path（CI 矩阵或 build tag + 本机）。
  - `$skill` 展开后 Session/derive 含 Skill 文本；load_skill 工具路径。
  - Todo 写入后 prepare/derive 可见。
  - ask：fixture 下 agent.request 往返后工具才执行。
  - 金路径：读 README → edit → shell `go test`（或 echo 代替）→ deny 路径。
- 验收：全量 `-count=1` 绿；`examples/coding.json` 文档可跟做。

## Out of Scope

- MCP（stdio/HTTP）、OS 级 sandbox、网络白名单、凭据屏蔽。
- 脚本 hooks 配置、插件 marketplace、完整 user/project/local settings 层级。
- PTY/交互 shell、流式逐步输出、git worktree 隔离。
- Plan Mode 工具子集、Anthropic 原生协议、自动 compact 策略（已有 hint）。
- Benchmark 套件（SWE-bench 等）——金路径测试之后另包。
- 并行执行写/shell 工具。

## Phase 2+ Backlog（只记录，不开票）

1. MCP client 插件（stdio tools → 星型）。
2. OS sandbox 与网络策略（独立安全模型，非 Interceptor）。
3. Hooks：生命周期 evt 订阅插件化。
4. 并行策略完善与 tool 搜索/延迟装载。
5. Benchmark harness。
6. settings 多层合并与托管策略。
7. async subagent 完整生命周期。

## Further Notes

- 术语以 `CONTEXT.md`（Workspace / Permission / Policy Capability / Read-only Tool / Skill / Skill Manager / Skill Trigger / Todo / Plan Constraint）为准。
- 约束：ADR-0001/0002/0014/0016/0017/0018/0019/0020。
- 建议实现顺序即 issues 01→10；02（tools 多提供方）是 05/08/09 的前置。
- 决策来源：grilling Q1–Q26（含 `$skill` 输入触发补充）。
