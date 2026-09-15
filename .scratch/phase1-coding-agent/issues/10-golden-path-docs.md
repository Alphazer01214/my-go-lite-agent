# 10 — 金路径测试、coding.json、文档

**What to build:**

- `examples/coding.json`：agent、session、llm-openai、context-manager、filetools、shelltools、permission、skill-manager、project-context。
- 主缝金路径测试（stub/fake llm + 真实工具插件）：
  1. Workspace 内 read → edit/write → shell（echo 或等价）→ 最终答复；
  2. policy deny write/shell 路径；
  3. `$skill` 展开；
  4. todo 投影；
  5. ask 往返（至少 CLI 或 Host 契约级）。
- README：特性表、coding 装配、Workspace/permissions/skills 目录约定、平台 shell 说明。
- 确认 CONTEXT/ADR 链接；`.scratch` 本 spec 勾完后 Status 保持/更新。

**Blocked by:** 01–09（实现票）

**Status:** ready-for-agent

- [x] coding.json
- [x] 金路径测试全绿
- [x] README/docs
- [x] 全量 `-count=1` 回归

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- Q10=先 test；Q11=B；Q19 顺序末票。
