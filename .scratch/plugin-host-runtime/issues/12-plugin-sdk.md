# 12 — Plugin SDK Serve/Call（二次开发 · 与原生同一路径）

**What to build:** 仓库内外同一套作者接口：`pluginsdk.Serve/Handle/Call/Emit`。原生插件（echo、consumer…）与第三方共用该包，Host 侧 `serve` 只做监督与星型路由，不提供第二套插件编程模型。

**Blocked by:** 03 — Assembly 挂载（04/05 的 Host 路由与生命周期已并行落地）

**Status:** resolved

- [x] `pluginsdk`：`Serve` 主循环、`Handle(cap, method)`、`Emit`、出站 `Call`（经 Host）
- [x] 未注册 method → `method_not_found` 结构化错误
- [x] `plugins/echo`、`plugins/consumer` 改为基于 SDK；集成测试仍绿
- [x] 包注释写明公开契约（Frame / manifest protocol=1）
- [x] 主缝：consumer 经 `Call` 打到 echo（星型）；echo 回环

## Answer

新增 `pluginsdk.Server`：stdin 分发 req 到 Handler，res 完成 pending `Call`，stdout 写回。`echo`/`consumer` 去掉手写 Frame 循环。Host `serve` 保持为内核侧进程监督 + 路由，与 SDK 分工：作者写 Handler，Host 管进程。

## Comments

- 后续 session/tool/llm/interceptor 一律 `pluginsdk`，禁止手写循环。
- Host 内默认 Loop 走 `serve.Server.Call`（进程内），cap/method/错误形状与 SDK 一致。
