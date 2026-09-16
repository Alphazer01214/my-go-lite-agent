# sessionprobe

> 状态：**测试夹具（不随 dist 发布）**。用于验证多会话 session.* 调用。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

一次调用创建两个 Session、各追加不同事实、分别 derive 并返回，验证 Session Log 与 Current Session 的隔离。

引用于：`cmd/liteagent-cli/multi_session_test.go`
