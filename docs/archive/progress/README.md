# 过程记录

本目录是 LiteAgent 的开发过程记录。它记录决策、调试过程和实现切片的中间状态。

它不是规范。平台契约在 [docs](../..) 根目录、插件契约在 `plugins/<name>/`。

本目录只增不改。历史记录写完就冻结，后续修正写在新的记录里，并在旧记录里标注取代者。

## 1. 状态标注

每篇记录的首行都有状态标注。读之前先看标注。

| 标注 | 含义 | 怎么用 |
|---|---|---|
| 当前实现 | 结论对当前代码仍然成立 | 可以直接照做 |
| 部分有效 | 主体成立，个别结论已过时 | 照做前先核对标注里点名的部分 |
| 已废止 | 结论已被取代 | 只作历史参考，不要照做 |
| 未采纳 | 设计没有落地 | 只作决策记录，不要照做 |
| 仅参考 | 执行证据或问题清单 | 查证据用，不要照做 |

## 2. 记录索引

### 2.1 仍有效的记录

| 文档 | 主题 |
|---|---|
| [2026-10-15-host-runtime-infrastructure-development-log.md](2026-10-15-host-runtime-infrastructure-development-log.md) | Plugin Instance、执行上下文、Generation 隔离。三条并发规则在此 |
| [2026-10-16-conversation-cancel-recovery-debug-log.md](2026-10-16-conversation-cancel-recovery-debug-log.md) | `turn_end` 缺失导致会话卡在 `running` |
| [2026-10-18-session-workspace-multichat.md](2026-10-18-session-workspace-multichat.md) | Session 恢复缺陷、Workspace 绑定、多会话标签 |
| [2026-10-19-webui-refresh-transport-plan.md](2026-10-19-webui-refresh-transport-plan.md) | Browser Call 与 HTTP 上下文解耦、多连接广播、hello/resync |
| [2026-10-20-full-test-fix-plan.md](2026-10-20-full-test-fix-plan.md) | P0 死锁链推理、U8 与取消语义的方案表 |
| [2026-10-20-model-context-tools-handoff.md](2026-10-20-model-context-tools-handoff.md) | model-context、tools、file-tools、shell-tools 的实现状态 |
| [2026-10-20-p0-write-lock-fix.md](2026-10-20-p0-write-lock-fix.md) | SDK 写锁拆分与本地超时 |
| [2026-10-20-phase2-p1-fix.md](2026-10-20-phase2-p1-fix.md) | 流式 outbox、失败 Turn 可见性、工作区口径 |
| [2026-10-20-p2-fix.md](2026-10-20-p2-fix.md) | 取消通知 Fact、慢连接关闭、错误文本清理 |
| [2026-10-21-sandbox-adjudication-implementation.md](2026-10-21-sandbox-adjudication-implementation.md) | sandbox 首批：三态裁决、规则集、审计与四个决策点 |

### 2.2 部分有效的记录

| 文档 | 主题 | 失效部分 |
|---|---|---|
| [2026-10-15-minimal-stack-debug-log.md](2026-10-15-minimal-stack-debug-log.md) | 最小组合的三个 bug | BUG-003 的 0600 权限修复已丢失 |
| [2026-10-15-webui-interaction-ux-improvement-log.md](2026-10-15-webui-interaction-ux-improvement-log.md) | Shell 交互优化与 B1 到 B9 重构 | B9 的 `evicted` 帧已删除 |
| [2026-10-16-plugin-center-session-ux-log.md](2026-10-16-plugin-center-session-ux-log.md) | 插件中心、依赖图、Session UX | 「启动前校验 `depends_on`」不成立 |
| [2026-10-19-webui-refresh-render-fix-log.md](2026-10-19-webui-refresh-render-fix-log.md) | 渲染层增量刷新 | 2.3 的 panes 集合规则已被回退 |

### 2.3 已废止的记录

| 文档 | 取代者 |
|---|---|
| [2026-10-14-webui-implementation-handoff.md](2026-10-14-webui-implementation-handoff.md) | [docs/platform/webui.md](../../platform/webui.md) |
| [2026-10-15-agent-llm-session-minimal-development-log.md](2026-10-15-agent-llm-session-minimal-development-log.md) | [plugins/session/communication.md](../../../plugins/session/communication.md) |
| [2026-10-15-management-plane-development-log.md](2026-10-15-management-plane-development-log.md) | [docs/platform/webui.md](../../platform/webui.md) |
| [2026-10-15-minimal-stack-handoff.md](2026-10-15-minimal-stack-handoff.md) | 本目录的开发日志与调试日志 |
| [2026-10-15-webui-browser-call-ui-push-handoff.md](2026-10-15-webui-browser-call-ui-push-handoff.md) | 2026-10-19 的传输层方案与 [docs/platform/webui.md](../../platform/webui.md) |
| [2026-10-15-webui-shell-loader-development-log.md](2026-10-15-webui-shell-loader-development-log.md) | [docs/platform/webui.md](../../platform/webui.md) |
| [2026-10-18-agent-framework-implementation.md](2026-10-18-agent-framework-implementation.md) | [2026-10-20-model-context-tools-handoff.md](2026-10-20-model-context-tools-handoff.md) |
| [2026-10-18-agent-tools-design-handoff.md](2026-10-18-agent-tools-design-handoff.md) | [2026-10-18-agent-tools-full-design.md](2026-10-18-agent-tools-full-design.md) |
| [2026-10-18-agent-tools-full-design.md](2026-10-18-agent-tools-full-design.md) | [docs/design/plugins/05-tools.md](../../design/plugins/05-tools.md) 与 [docs/design/composition-model-context-tools-sandbox.md](../../design/composition-model-context-tools-sandbox.md) |
| [2026-10-19-webui-refresh-render-debug-log.md](2026-10-19-webui-refresh-render-debug-log.md) | 2026-10-19 的渲染层修复记录 |

### 2.4 未采纳与仅参考

| 文档 | 主题 | 说明 |
|---|---|---|
| [2026-10-15-plugin-runtime-management-handoff.md](2026-10-15-plugin-runtime-management-handoff.md) | 插件启停、持久化、依赖级联 | 设计没有实现。保留了被否决的方案和五个开放决策 |
| [2026-10-20-full-test-report.md](2026-10-20-full-test-report.md) | 全量测试的执行证据 | 问题清单尚未全部关闭 |
| [2026-10-21-pressure-execution-records.md](2026-10-21-pressure-execution-records.md) | U9、U10、S11、S12 的压力测试执行记录 | 规程以根目录 PRESSURE.md 为准 |

## 3. 归档规则

记录只增不改。要废止一篇，就在它的状态标注里写清取代者，并在第 2 节把它移到「已废止」表。

不要因为记录过时就删掉它。设计上的取舍和被否决的方案只在过程记录里留底，规范文档不保留这些内容。
