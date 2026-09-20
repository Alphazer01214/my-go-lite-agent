# cmd/liteagent-cli — CLI 入口与集成测试

## 职责

CLI Render Medium 的进程入口。产品逻辑几乎全在 [internal/app](internal-app.md)；本目录 `main.go` 仅两行调用 `app.CLI()`。

**主要质量资产**是进程边界集成测试套件（`package main_test`，约 48 个 `Test*`）。

## 结构

```text
cmd/liteagent-cli/
  main.go              # package main + app.CLI()
  *_test.go            # 集成测试 + fixtures
  sessions/            # 测试会话数据目录（运行时）
```

## 入口

```go
func main() { app.CLI() }
```

产品 flag 见 [internal/app](internal-app.md#cli-medium-与-adr0029)。

## 测试基建（helpers_test.go）

| 设施 | 作用 |
|------|------|
| `buildPkg` | 进程内缓存 `go build` Host 与出厂插件二进制 |
| `hostEnv` | `SESSION_DATA_DIR=<temp>`、`L0_TEST_COMPAT=1` |
| `markAutostart` | 向 fixture Manifest 注入 `autostart: true`（走产品路径 ADR-0021） |
| `buildStubLLMPluginDir` | 确定性 LLM stub：流式 reasoning/content；有 tools 时发 tool_calls |
| `invokeArgs`（`l0_invoke_test.go`） | L0 `-invoke` 诊断驱动 |

## 测试覆盖簇

| 簇 | 主题 |
|----|------|
| Discovery | 合法/非法、UI-only、空壳 Manifest |
| Assembly | dump、Autostart 闭包 |
| REPL | 多 Turn 同一 Session |
| Commands | `/help`、建议、`/session dump-trace` |
| Loop | 一次 Turn + derive；缺 LLM 诊断 |
| Session 不变量 | 未记日志拒绝、空日志、空 claim；tool_call_id |
| Context | stub、compactHint、active Summary、prepare/compact/skill |
| Workspace | 绑定 + filetools 根 |
| Policy | medium severity ask、项目规则 deny |
| Config | `/refresh` 广播、llm-openai 持久化 |
| Scheme | agent scheme 切换 |

这些测试是 **Host↔插件协议与 Loop/Session/Context 行为的接缝套件**，优先于 mock。

## 运行

```powershell
go test ./cmd/liteagent-cli/ -count=1
```

需要本机可 `go build` 出厂插件；Windows/macOS/Linux 皆可。真实 OpenAI Turn 通常被环境门控跳过。

## 开发约定

1. 不要把产品逻辑下沉回 `cmd/`；保持薄 main。
2. 新集成场景优先扩展本套件（现场 build 夹具），不改成纯 mock。
3. 测试可依赖领域 flag（`L0_TEST_COMPAT`）；产品二进制不可见。
4. 遵守变更纪律：逻辑单元结束跑 `go build ./... && go vet ./... && go test ./...`。
