# crashonce

> 状态：**测试夹具（不随 dist 发布）**。用于验证插件崩溃后的按需重启。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

进程首次启动即退出，之后行为等同 `echo`；用于验证 Host 的 `ensureAlive` 重启与 `plugin_down` 重试。

引用于：`cmd/liteagent-cli/lifecycle_fixtures_test.go`
