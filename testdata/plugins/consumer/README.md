# consumer

> 状态：**测试夹具（不随 dist 发布）**。用于验证星型路由与 consumes 未满足时的行为。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

提供 `demo` Capability，且只经 Host 调用另一个 Capability（不直连）。

引用于：`cmd/liteagent-cli/routing_test.go`、`lifecycle_test.go`
