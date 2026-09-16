# 02 — sandbox severityPolicy

**What to build:** `policy.decide` 接受可选 `severity`；无显式规则时按可配置 `severityPolicy`（默认 low→allow, medium→ask, high→ask）裁决；Config 暴露/可写该映射。

**Status:** ready-for-agent

- [x] decide(severity)
- [x] severityPolicy config
- [x] rules 仍优先 deny>ask>allow
