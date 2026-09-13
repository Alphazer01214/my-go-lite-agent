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
- **`llm-openai`**：OpenAI 兼容适配（DeepSeek 等），流式输出
- **REPL**：`-repl` 多轮同 Session，实时流式打印
- **Presentation**：`stream` / `status` / `card` 三类信号，CLI 为默认 Render Medium
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
  liteagent-cli.exe     CLI Medium（REPL / -turn / session ops / 诊断）
  liteagent-server.exe  Web Medium（-serve，可与 -repl 组合）
  plugins/
    session/           memory Session Log
    llm-openai/        OpenAI 兼容 LLM（DeepSeek 等）+ config.example.json
    fakellm/           测试用假模型
    contextmanager/    system-prompt 组装 + segments.json
    filetools/         读写/grep/glob
    echotool/          演示工具 + Presentation Card
    echo/ interceptor/
  examples/
    chat.json          session + llm-openai + contextmanager
    agent.json         chat + filetools
    assembly.json      fakellm 最小集（测试）
    assembly-with-tools.json
```

### 真实模型（DeepSeek / OpenAI 兼容）

```powershell
# 方式一：环境变量（优先）
$env:OPENAI_API_KEY = "sk-..."
$env:OPENAI_BASE_URL = "https://api.deepseek.com/v1"   # 可省略，默认 DeepSeek
$env:OPENAI_MODEL = "deepseek-chat"                   # 或你账户可用的模型名

# 方式二：复制配置到插件目录
copy plugins\llm-openai\config.example.json plugins\llm-openai\config.json
# 编辑 config.json 填入 apiKey / model

cd dist
.\liteagent-cli.exe -plugins plugins -assembly examples\chat.json -repl
```

输入多轮对话；`/help` 查看命令；`/` 后按 Tab 可补全命令/插件名；`/exit` 或 Ctrl+C 退出。

渲染默认全量展示（无需 `-verbose`）：

- **Thinking…** — 尚无正文时的淡化占位（`message_text` dim）
- **Generating… N chars** — 流式过程中的单行进度；正文在 settle 后以 Markdown→ANSI 完整渲染
- **markdown_text** — 助手正文（标题/列表/代码块/表格/链接）
- **summary_text** — 工具调用摘要卡（`⏺ 工具名` + 键值参数 + 截断详情）
- **message_text** — 状态/错误行

插件可通过 `pluginsdk.EmitMarkdownText` / `EmitMessageText` / `EmitSummaryText` 向 Render Medium 发分类意图。

带文件工具：

```powershell
.\liteagent-cli.exe -plugins plugins -assembly examples\agent.json -repl
```

### Web Medium（浏览器）

```powershell
.\liteagent-server.exe -plugins plugins -assembly examples\chat.json -serve 127.0.0.1:7788
# 可与终端 REPL 并存：
.\liteagent-server.exe -plugins plugins -assembly examples\chat.json -serve 127.0.0.1:7788 -repl
```

打开 `http://127.0.0.1:7788`：

- Host **Shell** 提供布局、默认聊天面（markdown / 工具卡 / 流式正文 / Thinking）与命令输入；**仅聊天区滚动**
- 侧栏链到 **`/trace`**：独立 Session 轨迹页（user / assistant / system / tool_call / tool_result / step…）
- Session 持久化：session 插件把事实写入 JSONL（默认 `./sessions/`，可用 `SESSION_DATA_DIR` 覆盖）；Host 重启后 `/api/history` 仍可回放
- 新标签页 SSE `?replay=1` 回放最近 presentation 事件

**Panel Component（插件业务 UI，ADR-0010）**——插件自带 html/js/css，Host 零 Web 渲染编码：

- Manifest 声明 UI Entry 与静态挂载：`"ui": {"entry": "main.js", "mounts": [{"slot": "sidebar", "component": "<插件名>-mode-panel", "props": {...}}]}`
- `ui/main.js` 是普通 ES Module：`customElements.define('<插件名>-…', …)`，组件用 Shadow DOM + Shell 的 `--la-*` Design Token 保持主题联动；零构建链
- **多文件资产**：`ui.assets` 声明组件运行时 fetch 的 css / `<template>` html 文件（相对 `ui/`，Discovery 校验存在）——html 写结构、css 写样式、js 写行为；组件内 `fetch(new URL('panels.css', import.meta.url))` + `adoptedStyleSheets` + 模板克隆。参考 `plugins/uidemo/ui/`
- 运行时变更走 `EmitPanel(PanelOp{op: set|clear, slot, id, component, props})`；Host 校验组件名必须以插件名为前缀
- 组件内部经全局 `LiteAgent` 回传：`emitUIAction(plugin, panel, event, value, props?)` → 插件的 `cap=ui, method=action`；`onSessionChange(fn)` 响应会话切换；`on(topic, fn)` 订阅 SSE；`call(cap, method, payload)` 直调 Capability
- `/refresh` 后插件变更由 Shell 整页刷新承接（状态真源在 Session Log）
- 作者 SDK：`GET /sdk/lite-agent.js`（与仓库 `sdk/lite-agent.js` 同源，可拷贝）

参考实现：`plugins/uidemo`（静态挂载 + Shadow DOM + UI Action 回传 + 动态 PanelOp + 会话徽标五面俱全）。

### 单发一轮（脚本友好）

```powershell
.\liteagent-cli.exe -plugins plugins -assembly examples\chat.json -turn "hello" -session-derive
```

### 无真实 Key 时（fixture）

```powershell
.\liteagent-cli.exe -plugins plugins -assembly examples\assembly.json `
  -turn "hello" -session-derive
```

### 发现与挂载

```powershell
# 只扫描，不挂载
.\liteagent-cli.exe -discover plugins

# 挂载并 dump Assembly 树
.\liteagent-cli.exe -plugins plugins -assembly examples\chat.json -dump
```

### Session / 不变量 / 注入

```powershell
# 追加事实并派生 Model Context
.\liteagent-cli.exe -plugins plugins -assembly examples\assembly.json `
  -session-append '[{"role":"user","content":"hi"}]' -session-derive

# 校验：claimed 必须能从 Session Log 重建，否则拒绝
.\liteagent-cli.exe -plugins plugins -assembly examples\assembly.json `
  -session-append '[{"role":"user","content":"hi"}]' `
  -agent-request '[{"role":"user","content":"hi"}]'

# 注入模型可见消息（不启动 Loop）
.\liteagent-cli.exe -plugins plugins -assembly examples\assembly.json `
  -agent-inject '[{"role":"system","content":"note"}]' -session-derive
```

### Waterfall 审计

```powershell
.\liteagent-cli.exe -plugins plugins -assembly examples\chat.json `
  -turn "audit me" -audit
```

## 插件目录布局

每个插件是一个目录：

```text
my-plugin/
  plugin.json     # 清单（必需）
  my-plugin.exe   # 可执行（entry；UI-only 插件可省，见 ui）
  ui/             # 可选：Panel Component 资产（ui.entry 指向的 ES Module）
  static/         # 可选静态文件（默认仅本插件可见）
  config.json     # 可选（llm-openai 等）
  segments.json   # 可选（contextmanager）
```

`plugin.json` 最小示例：

```json
{
  "name": "session",
  "version": "0.1.0",
  "protocol": 2,
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
| `entry` | 相对本目录的可执行文件名；与 `ui` 至少其一（UI-only 插件无 exe、无进程、不占 Capability，仅提供 Web UI） |
| `timeoutMs` | 可选，单次调用超时（默认 30000） |
| `ui` | 可选 Web UI 声明：`entry`（ES Module）、`assets`（css/html 资产，声明即校验存在）、`mounts`（静态挂载） |

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

主缝测试在 `cmd/liteagent-cli`（CLI 面）与 `cmd/liteagent-server`（Web 面）：真实 Host 可执行 + fixture 插件进程，断言外部可观察行为。

规格与工单：`.scratch/plugin-host-runtime/`。

## 发布说明（v0.1）

- 平台：Windows amd64（进程模型按 Windows 验证）
- 依赖：无第三方运行时库（仅 Go 标准库）
- LLM：附带 `fakellm` 本地假模型；接真实提供商请实现 `llm.complete` 插件
- 范围：单机 stdio 插件；无 HMR、无远程插件、无图形 UI

## 许可

按仓库根目录许可文件（如有）执行。
