# emptytools

> 状态：**测试夹具（不随 dist 发布）**。用于验证「挂载了 tools 但没有任何模型可见工具」的场景。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

Tools Plugin：`list` 返回空数组，`call` 一律报错。Subagent 场景下不应因它产生 Todo。

引用于：`cmd/liteagent-cli/subagent_test.go`、`serve/tools_conflict_test.go`
