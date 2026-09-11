# my-go-lite-agent

轻量 Go Agent 运行时：**Host 薄内核 + 进程外插件**。拿到二进制与插件目录即可运行、可换能力，不要求阅读源码，核心零第三方依赖。

```text
用户输入
   │
   ▼
┌──────────── Host（薄内核）────────────┐
│  Discovery · Assembly · 生命周期       │
│  Frame 路由（星型）· 双层 Waterfall    │
│  Session 不变量 · 默认 Agent Loop      │
└───┬──────────┬──────────┬──────────┬──┘
    │          │          │          │
 session    llm        tools    interceptor
（插件）  （插件）    （插件）     （插件）
```

规范名词见 [CONTEXT.md](CONTEXT.md)；架构决策见 [docs/adr/](docs/adr/)。

## 特性

- **进程外插件**：stdin/stdout 上的长度前缀 JSON Frame；崩溃隔离、可独立分发
- **Discovery ≠ Assembly**：扫描看见插件，配置决定挂载；未点名不拉起
- **星型路由**：插件之间不直连，策略平面唯一
- **双层 Waterfall**：内建审计/取消始终生效；外部 Interceptor 可放行/改写/短路
- **Session Log 不变量**：仅追加日志是历史唯一真源；模型可见内容必须可从日志重建
- **默认 Agent Loop 在 Host**：开箱跑通一轮对话；可用外置 `loop` 插件替换
- **Presentation Card**：工具可发纯函数投影的结构化卡片（可回放）
- **Windows 一等公民**：进程模型按 Windows 语义验证

## 快速开始

### 构建

```powershell
# Windows PowerShell
.\scripts\build.ps1
```

产物在 `dist/`：

```text
dist/
  host.exe
  plugins/
    session/     session.exe + plugin.json
    fakellm/     fakellm.exe + plugin.json
    echotool/    echotool.exe + plugin.json
    echo/        echo.exe     + plugin.json
    interceptor/ interceptor.exe + plugin.json
  examples/
    assembly.json
    assembly-with-tools.json
```

### 跑一轮对话（最小 Assembly）

```powershell
cd dist
.\host.exe -plugins plugins -assembly examples\assembly.json `
  -turn "hello" -session-derive
```

期望输出包含：`turn ok`、流式 `chunk[...]`、`derive ok messages=[...]`。

### 带工具的一轮

```powershell
.\host.exe -plugins plugins -assembly examples\assembly-with-tools.json `
  -turn "please echo me" -session-derive -cards
```

会看到 `tool_call`、最终 `Tool said: ...`，以及 `card[0] type=echo_result`。

### 发现与挂载

```powershell
# 只扫描，不挂载
.\host.exe -discover plugins

# 挂载并 dump Assembly 树
.\host.exe -plugins plugins -assembly examples\assembly.json -dump
```

### Session / 不变量 / 注入

```powershell
# 追加事实并派生 Model Context
.\host.exe -plugins plugins -assembly examples\assembly.json `
  -session-append '[{"role":"user","content":"hi"}]' -session-derive

# 校验：claimed 必须能从 Session Log 重建，否则拒绝
.\host.exe -plugins plugins -assembly examples\assembly.json `
  -session-append '[{"role":"user","content":"hi"}]' `
  -agent-request '[{"role":"user","content":"hi"}]'

# 注入模型可见消息（不启动 Loop）
.\host.exe -plugins plugins -assembly examples\assembly.json `
  -agent-inject '[{"role":"system","content":"note"}]' -session-derive
```

### Waterfall 审计

```powershell
.\host.exe -plugins plugins -assembly examples\assembly.json `
  -turn "audit me" -audit
```

## 插件目录布局

每个插件是一个目录：

```text
my-plugin/
  plugin.json     # 清单（必需）
  my-plugin.exe   # 可执行（entry）
  static/         # 可选静态文件（默认仅本插件可见）
```

`plugin.json` 最小示例：

```json
{
  "name": "session",
  "version": "0.1.0",
  "protocol": 1,
  "provides": ["session"],
  "consumes": [],
  "entry": "session.exe",
  "timeoutMs": 30000
}
```

| 字段 | 说明 |
|------|------|
| `name` | 插件名，Assembly 点名用 |
| `version` | 版本字符串 |
| `protocol` | 必须为 `1` |
| `provides` | 对外 Capability 列表 |
| `consumes` | 启动前必须被满足的 Capability |
| `entry` | 相对本目录的可执行文件名 |
| `timeoutMs` | 可选，单次调用超时（默认 30000） |

## Assembly 配置

```json
{ "plugins": ["session", "fakellm"] }
```

只挂载点名的插件；引用不存在的名字会 fail-loud。

## 写一个插件

使用仓库内 `pluginsdk`（与内置插件同一路径）：

```go
package main

import (
    "encoding/json"

    "github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
    s := pluginsdk.New()
    s.Handle("demo", "ping", func(req *pluginsdk.Request) (json.RawMessage, error) {
        return json.RawMessage(`{"pong":true}`), nil
    })
    // 经 Host 调用其他 Capability（星型，禁止直连）
    // out, err := s.Call("session", "derive", json.RawMessage(`{}`))
    // 发 Presentation Card（纯投影 + Emit 传输）
    // _ = s.EmitCard(pluginsdk.Card{CardType: "demo", Data: ...})
    _ = s.Serve()
}
```

约定：

- **Function** 一次调用返回 `result` / `additionalContexts`，不编排循环
- **异步模型可见通知** 走 `agent.inject`，不要改写历史
- **Presentation Card** 投影必须是 args/result 的纯函数（无 I/O、时钟、随机）

## 内置 Capability（Host 侧）

| Capability | 说明 |
|------------|------|
| `agent.request` | 校验 Model Context 可从 Session Log 重建 |
| `agent.inject` | 追加模型可见消息，不启动 Loop |
| 默认 Loop | `session` + `llm`（+ 可选 `tools`）驱动一轮对话 |

插件提供的 Capability 示例：`session`、`llm`、`tools`、`interceptor`、`echo`。

## 开发

```powershell
go test ./...
go vet ./...
```

主缝测试在 `cmd/host`：真实 Host 可执行 + fixture 插件进程，断言外部可观察行为。

规格与工单：`.scratch/plugin-host-runtime/`。

## 发布说明（v0.1）

- 平台：Windows amd64（进程模型按 Windows 验证）
- 依赖：无第三方运行时库（仅 Go 标准库）
- LLM：附带 `fakellm` 本地假模型；接真实提供商请实现 `llm.complete` 插件
- 范围：单机 stdio 插件；无 HMR、无远程插件、无图形 UI

## 许可

按仓库根目录许可文件（如有）执行。
