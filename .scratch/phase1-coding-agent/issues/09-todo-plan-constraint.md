# 09 — Todo 工具与 CM 投影、Plan Constraint 提示段

**What to build:**

- **agent 内建** `todo` 工具（非独立插件）：update/list 语义最小集（写入当前计划条目状态：pending/in_progress/done 等）；事实 `session.append`（meta 可辨识 todo）。
- **context-manager**：`context.prepare` 从 Session Log 投影**最新** Todo 列表为 Model Context 可见段（或 user/system 附属段，保持可重建）。
- **Plan Constraint**：CM 静态/装配 Prompt Segment：指导「先计划、用 todo、再改文件」；**不**裁剪 tools、无 mode 切换。
- readOnly：若 todo 含只读 list 工具可标 `readOnly: true`（可选）。

**Blocked by:** 02；投影依赖 session derive 语义（已有）

**Status:** ready-for-agent

- [x] todo 工具 + Log 事实
- [x] prepare 投影
- [x] Plan Constraint 段
- [x] 主缝：写 todo → 下一 prepare/derive 可见

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- Q9/C、Q17（无门禁）、Q24=C。
