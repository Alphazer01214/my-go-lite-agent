# slow

> 状态：**测试夹具（不随 dist 发布）**。用于验证调用超时。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

应答前故意延迟；用于校验 Host 的 per-call timeout 与超时后的错误帧。

引用于：`cmd/liteagent-cli/lifecycle_test.go`
