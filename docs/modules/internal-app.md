# internal/app — 双入口共享 Medium 运行时

规范名词：[Render Medium](../../CONTEXT.md)、[CLI Medium](../../CONTEXT.md)、[Command](../../CONTEXT.md)、[Host](../../CONTEXT.md)。相关 ADR：0008、0011、0020、0021、0029、0030。

## 职责

`liteagent-cli` 与 `liteagent-server` 共用的 Medium 层：flag 分发、Assembly 装载助手、CLI 绘制、REPL、Command 面、Web 启动器。

两个二进制只在启动哪个 Render Medium 上分叉；Host 与装配逻辑只有一份（ADR-0011/0029）。

对外仅暴露：`CLI()`、`Server()`。

## 文件职责

| 文件 | 角色 |
|------|------|
| `app.go` | `resolveAssembly`（Autostart 闭包；`-assembly` 废弃）、`startMounted`、Discovery 打印、roundtrip 探针、`runCallPlugin`、`probeCommandFaces` |
| `cli.go` | `CLI()` 入口与 L0 产品 flag |
| `server.go` | `Server()` 入口（web） |
| `web.go` | `runWebAndOptionalREPL`：装配 → Layout 合并 → 挂载 → workspace 种子 → CommandPlane → `web.New` |
| `repl.go` | `runREPL` / `runREPLLoop`：approval 注册、SIGINT、banner、readline |
| `paint.go` | `turnRenderer`（CLI 绘制）+ `cliToolApproval` |
| `callx.go` | `callPlugin` / `runLoopTurn` / `cancelLoopTurn`（点名 agent） |
| `invoke.go` | `-invoke` 诊断 |
| `commands.go` | Command 面（原生 + 插件 hostFace） |
| `readline.go` | raw-mode 行编辑、Tab 补全 |
| `vt_windows.go` / `vt_other.go` | Windows VT 开关 |
| `legacy_compat.go` | `L0_TEST_COMPAT=1` 时的测试用领域 argv |

## CLI Medium 与 ADR-0029

决策与时机逻辑在 Host 一次；展示后端可多注册：

- **Approval**：`RegisterApproval` 追加面；`askApproval` 并行、先到先得
- **展示订阅**：`wireRenderer` 在单 Turn 期间订阅 stream/status/presentation
- **`-serve -repl` 同开**：两个 approval 面并存，不互相覆盖
- **webCommandPlane**：同一 commandPlane 适配 Web；`cp.web` 过滤 CLI 专属帮助

## Presentation 消费（CLI）

1. Turn 前：`wireRenderer` + `r.begin()`
2. Host `Event{Topic,Data}`：stream / status / presentation
3. 瞬态（Thinking / 流式预览）不替代已提交 Render Intent
4. `onRender`：清瞬态后按 kind 绘制（markdown → `mdansi.Render`）
5. `r.end`：若未画过内容则 markdown 兜底
6. restore 取消订阅——展示是 Turn 级，不是永久单槽

## Command 面（ADR-0008）

| 类型 | 路由 |
|------|------|
| 原生 | `/help` `/lp` `/refresh` `/exit`（固定集合） |
| 插件 | `/<plugin> [cmd] [args]` → `CallByFace(..., "commands", "call", ...)` |

- 与原生同名的插件**不予加载**
- 未知命令：编辑距离 ≤2 建议
- `/refresh`：重扫 Discovery，保留已挂载集，只对声明 `config` face 的插件广播 `config.reload`
- `/exit` 仅 CLI；Web `/help` 隐藏

## 启动装配

```text
CLI()/Server()
  → enableVirtualTerminal()
  → flag.Parse（L0_TEST_COMPAT 可挂 legacy 域 flag）
  → resolveAssembly: discovery.Scan → assembly.ResolveAutostart
  → startMounted: serve.Start + SetCatalog + SetPluginsDir
  → （可选）session.create workspace 种子（软，ADR-0020）
  → RegisterApproval(cliToolApproval)
  → probeCommandFaces
  → commandPlane
  → runREPLLoop | runWebAndOptionalREPL | 一次性 invoke/call
```

产品 argv 仅 L0：`-plugins` `-repl` `-serve` `-debug` `-discover` `-dump` `-invoke` `-call-plugin` `-layout` 等。领域 flag 仅测试兼容路径。

## Windows VT

`enableVirtualTerminal`：`ENABLE_VIRTUAL_TERMINAL_PROCESSING`，否则流式 `\r\x1b[2K` 在 conhost 上变成字面量。启动任何绘制前调用。

## 已知残留 L1（ADR-0030 收敛中）

生产 Medium 仍含少量领域点名：`session.create` workspace 种子、`runLoopTurn` 点名 `agent`/`loop`、`agentSchemeLabel` 经 agent-presets、`printInvokeResult` 特例。产品 argv 已 L0-only；其余待下放插件。

## 开发约定

1. 不要在 Medium 自建第二条事件总线或单槽 approval。
2. 绘制可用 pluginsdk 展示类型；不要编排 session/loop 业务状态机。
3. 不要恢复领域启动 flag（无 `L0_TEST_COMPAT` 时）。
4. 装配错误软失败：打印、继续（ADR-0017）。
