# 08 — skill-manager：发现、CM 联动、$skill、load_skill

**What to build:** 新建 `plugins/skill-manager`：

- 发现：扫描 `<Workspace>/.liteagent/skills/<name>/SKILL.md`。
- 目录：向 CM `registerSkill`（名 + 简介），目录进 System Prompt（沿用 CM 占位）。
- 全文：工具 `load_skill`（模型可调用）读入 SKILL.md；结果进 tool result / additionalContexts。
- **Skill Trigger**：提供方法供 agent 在 Turn 开始展开用户输入中的 `$name`（独立 token；未知名返回可注入的错误说明或原样保留+告知）。
- 不实现 agentskills.io 全兼容；布局以本票为准。
- 可选 slash：`/skill-manager list`。

**Blocked by:** 02（多 tools）、01（workspace）

**Status:** ready-for-agent

- [x] 目录发现 + registerSkill
- [x] load_skill
- [x] $name 展开契约（供 04 调用）
- [x] 主缝：注册后目录进 prepare；$ 展开后文本可见；load_skill 路径

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- Q8=B、Q16=B、Q25=B；用户补充：`$skill` 输入阶段解析。
- 与 04 的展开接口对齐方法名。
