# agentprobe

> 状态：**测试夹具（不随 dist 发布）**。用于验证 Host `agent.request` 的 Session Log 不变量。构建脚本（`scripts/build.sh` / `scripts/build.ps1`）不安装它；Go 测试在临时目录里按需编译本包。

## 用途

经星型调用 Host 的 `agent/*`，`payload.cap` 决定分支：

- 省略 → `agent/request` 空声明（Host 必须从 Session Log 重建）
- `smuggle` → 带未落日志的 messages 调 `agent/request`（Host 必须拒绝）
- `inject` → `agent.inject` 一条 system 备注（只追加，不唤醒 Turn）

引用于：`cmd/liteagent-cli/context_test.go`、`session_test.go`、`lifecycle_fixtures_test.go`
