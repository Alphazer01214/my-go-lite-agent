# 01 — Frame 回环：Host + echo 插件

**What to build:** 用户能启动 Host，由 Host 拉起一个示例 echo 插件进程，经 stdin/stdout 完成一次 Frame `req`/`res` 回环，随后干净退出。建立 go module 与 Host 进程边界主缝测试的最小闭环。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 存在可启动的 Host 入口，按给定插件 entry 拉起子进程并接管其 stdio
- [x] Frame 为 `uint32` 长度前缀 + JSON body，含 `v/id/type/cap/method/payload/error` 字段（见 spec）
- [x] echo 插件能对任意 `req` 回一条对齐 `id` 的 `res`
- [x] Host 在收到预期 `res` 后正常退出；插件管道关闭后无残留子进程
- [x] 集成测试在 Windows 上可跑，只断言外部可观察行为（启动、帧内容、退出码）

## Answer

实现了 `protocol`（长度前缀 Frame 编解码）、`cmd/host`（`-plugin` 拉起子进程并完成一次回环）、`plugins/echo` fixture。主缝集成测试在 `cmd/host`：构建 host+echo 后运行并断言输出与退出码。`go test ./...` 与手工 `host -plugin echo` 均通过（exit=0）。

## Comments

- 外部测试包对 `package main` 必须命名为 `main_test`（曾误用 `host_test` 导致 setup failed）。
