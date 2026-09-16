# 06 — Subagent 同步路径

**What to build:** 父 Agent 经同步 tool call 触发 Subagent：新 Agent 实例 + 新 Session，跑完整 Turn，最终 assistant 文本作为 tool_result 回父。调用方可声明工具子集与 System Prompt 片段；声明异步时 v1 可先拒绝或降级同步。

**Blocked by:** 01 — Context Manager，05 — multi-session

**Status:** resolved

- [x] Host/Loop 支持「spawn subagent」：创建子 Session + 子 Loop 执行（内建默认 Agent）
- [x] tool call 载荷：input/text、可选 tools 子集、可选 systemPrompt 片段、可选 mode=sync|async
- [x] v1：mode=sync 完整实现；mode=async 返回 `not_supported`
- [x] 子 Session 元数据：origin=subagent、delegationDepth
- [x] 子产出以 tool_result 回父；子日志不混入父 derive
- [x] 主缝测试：父调 run_subagent 工具 → 子独立完整 turn → 父 tool_result 含子最终答复
- [x] 越权：子不继承 run_subagent（防递归）；tools 子集过滤留 TODO（见 Comments）

## Answer

Host 注入模型工具 `run_subagent`（仅当挂载了 tools 插件且允许 subagent，追加在 tools.list 之后以免抢走 fixture 首选工具）。`CallTool` 拦截该名并调用 `RunSubagent`：`session.create`（origin=subagent）→ 可选 system 事实 → `runTurn(childID, input, allowSubagent=false)` → 返回 assistant 文本。`mode=async` 返回 `not_supported`。arguments 兼容 `input`/`text`。主缝测试 `TestSubagentSyncToolResult`（session+fakellm+emptytools）；`emptytools` fixture 使 schemas 仅含 run_subagent。

## Comments

- 对齐 Q12=A、Q13=调用时声明同步/异步、Q14=父显式子集否则继承。
- **tools 子集过滤**：`toolFilter` 参数已预留，v1 子 Turn 仍用全局 tools.list；完整过滤待后续。
- 异步 + Inbox/steer 属后续 feature（spec Out of Scope）。
- 取消传播（父取消 → 子取消）未做，先保证同步完成路径。
- 无 tools 插件时不注入 run_subagent，保持最小 demo 无多余 tool-call。
