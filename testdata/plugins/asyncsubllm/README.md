# asyncsubllm

> 状态：**测试夹具（不随 dist 发布）**。用于验证异步 Subagent 的回注路径。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

夹具 LLM：第一跳要求 `run_subagent mode=async`，后续跳返回最终答复，用来验证「先返回子 Session id，完成后经 `agent.inject` 回注」。

引用于：`cmd/liteagent-cli/subagent_test.go`
