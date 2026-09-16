# 05 — 拆除 Waterfall / Interceptor

**What to build:** 删除外部 Interceptor 与双层 Waterfall 机制，保持星路由直接放行：

- 代码：`serve/waterfall.go`、routeRequest 接线、`InterceptorCap`、相关常量
- 夹具：`plugins/interceptor`、`plugins/crashix`（若仅为此存在）
- 测试与 CLI `-audit`（若无保留价值一并删）
- 文档：CONTEXT.md 的 Waterfall / Interceptor、Host 定义句、README、ADR 中相关表述或加 superseded 说明

**Blocked by:** —（可与 01–04 并行，勿混入同一 PR）

**Status:** resolved

- [x] 代码与夹具删除干净，无死配置
- [x] 星路由在无 interceptor 时行为等同 allow
- [x] CONTEXT.md 术语移除
- [x] 全量测试绿

## Answer

删除 `serve/waterfall.go`、`plugins/interceptor`、`plugins/crashix`、`waterfall_test.go` 与夹具构建函数；`routeRequest` 仅保留 `host_closed` 拒绝；去掉 `-audit` / `printAudit` / `Server.audit`；CONTEXT.md 删除 Waterfall/Interceptor 并改 Host 定义；ADR-0001 交叉引用 ADR-0014；README 改为 context 用量示例。

## Comments

- 对齐 Q15/Q21：横冲直撞阶段；默认 Loop 本就不经过 Waterfall。
- 保留 host_closed 检查可作普通 if，不必保留 Interceptor 概念。
