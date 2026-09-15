# 03 — permission 插件：policy Capability 与双层配置

**What to build:** 新建 `plugins/permission`：

- provides `policy`；方法 `decide`（及可选 `rules` 观测）。
- 入参：tool 名、关键 arguments（path/command 等）、workspace；出参：`allow|ask|deny` + reason。
- 规则文件：
  - 默认：`<exe 旁>/config/permissions.json`（与 dist 布局对齐，缺文件则空规则）。
  - 覆盖：`<Workspace>/.liteagent/permissions.json`。
  - 合并：项目层字段覆盖默认层；规则数组以项目层为准（浅语义，spec 已定）。
- 规则形状（建议）：`{"rules":[{"tool":"shell|write_file|edit_file|*","path":"src/**","action":"deny|ask|allow"}]}`；匹配序 **deny > ask > allow**；未匹配默认 allow（或可配置 defaultAction）。
- Config Capability：get/set 可改运行时动作与 defaultAction；reload 读盘。
- 无 Workspace 时拒绝项目层、只用默认层。

**Blocked by:** 无（建议 01 先行以便项目层路径真实）

**Status:** ready-for-agent

- [x] policy.decide 契约与实现
- [x] 双层文件加载与覆盖语义
- [x] 主缝：deny 命中、项目覆盖默认、缺文件默认放行

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- ADR-0019；Q3/Q13/Q22。命名 Capability=`policy`，词条 Permission。
