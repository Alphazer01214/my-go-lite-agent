# 03 — Request Header 快照

**What to build:** 每次模型调用前把 Request Header（provider/model/采样参数等）作为 log-only 事实落入 Session Log，便于审计与重放。

**Blocked by:** 02 — Turn/Step 边界事件

**Status:** resolved

- [x] Fact type `request_header`：meta 携带 provider、model、以及可选 temperature/maxTokens 等
- [x] v1 无真实 provider 时：header 来自 Loop 默认值（provider/model = default）
- [x] `session.derive` 忽略 `request_header`
- [x] 主缝测试：一轮结束后 query 到 request_header；derive 不含它

## Answer

默认 Loop 在每次 `llm.complete` 前 append `request_header`（role=host，meta: provider/model=step/turn）。v1 provider/model 写死 `default`，真实 adapter 立项时再读插件声明。derive 原本不投影该 type。主缝测试 `TestRequestHeaderSnapshot`。

## Comments

- 对齐 ADR-0005：header 落盘不等于两阶段 prepareCall；方法拆分仍 out of scope。
- 字段集 v1 从窄：provider + model + step + turn；多余字段后续放 meta 透传。
