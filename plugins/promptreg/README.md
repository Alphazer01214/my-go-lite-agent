# promptreg

> 状态：**测试夹具（不随 dist 发布）**。用于验证运行时注册 Prompt Segment。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

经星型调用 `system-prompt.registerSegment`，`payload` 形如 `{"text":"...","name":"...","order":N}`。

引用于：`cmd/liteagent-cli/context_manager_test.go`
