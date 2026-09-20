# 01 — Workspace：Session 元数据与 CLI/Server 接入

**What to build:** 定义并落地 Workspace：

- Session 元数据增加 `workspace`（绝对路径）；空值语义明确（建议：创建时必填或默认 cwd 一次性写入）。
- CLI：启动/建会话默认进程 cwd；可选 `-workspace` 覆盖。
- Server/Web：会话创建/选择 UI 提供目录输入或列表（最小可用：文本路径 + 校验存在）；写入该 Session 元数据。
- Subagent 继承父 Session Workspace（除非显式覆盖，v1 可只继承）。
- 暴露只读查询：session 侧或 Host 命令可查看当前 Workspace。

**Blocked by:** 无

**Status:** resolved

- [x] Session 元数据契约与 derive/query 可见
- [x] CLI 默认 cwd + flag
- [x] Server 会话 Workspace 写入路径
- [x] 主缝测试：两 Session 不同根互不串扰（可先用元数据断言）

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- ADR-0020；Q12/Q21。
- 工具侧读取在 05/06，本票只保证真源与接入。
