# 02 — sandbox severityPolicy

**What to build:** `policy.decide` 接受可选 `severity`；无显式规则时按可配置 `severityPolicy`（默认 low→allow, medium→ask, high→ask）裁决；Config 暴露/可写该映射。

**Status:** resolved

- [x] decide(severity)
- [x] severityPolicy config
- [x] rules 仍优先 deny>ask>allow

## Answer

refactor-host-boundary Z1 对账确认：本票内容已在既有实现中落地（见上方勾选项
与对应 ADR/测试），状态由 ready-for-agent 滞后修正为 resolved，非本轮新开发。
