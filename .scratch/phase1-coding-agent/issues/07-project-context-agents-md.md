# 07 — project-context：AGENTS.md → Prompt Segment

**What to build:** 新建 `plugins/project-context`（或并入 skill-manager——**默认独立小插件**）：

- 启动/Turn 前读 Workspace 根：`AGENTS.md`，缺省回退 `CLAUDE.md`；二者皆有时优先 `AGENTS.md`（或拼接，实现时定死一种并测）。
- 经 system-prompt/context 注册为 **Prompt Segment**（name 如 `project-context`）。
- 文件变更后 `/refresh` 或 config reload 可重读；v1 不必 watch。
- 超长截断上限，对齐 CM 其它段。

**Blocked by:** 01

**Status:** ready-for-agent

- [x] 读根文件 → Segment
- [x] 主缝：derive/assemble 含文件内容标记

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- Q7=B；词条用 Prompt Segment，不引入第二套 prompt 管道。
