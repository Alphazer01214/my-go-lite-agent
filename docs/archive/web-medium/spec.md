# Web Medium（Shell + Panel 注入）

Status: ready-for-agent

## Problem Statement

CLI 已是默认 Render Medium，但需要浏览器 GUI：agent/chat 模式选择、会话轨迹等业务 UI 应由插件提供，Host 只做基础壳。约束：lite agent，**前端零 Node**；作者完全可信；渲染过程是「注入」而非沙箱 iframe。

## Solution

### 1. 拓扑

- Host 内嵌 `net/http`：`host -serve :7788`（可与 `-repl` 并存，事件 fan-out）。
- 星型不变：浏览器不直连插件进程；一切经 Host。
- protocol **仍为 2**：新增 `presentation.panel` method 与 `ui` cap；旧消费者忽略未知 method。

### 2. Shell（Host 原生，不可被插件替换）

- 页面布局 + 三个固定 Panel 槽：`sidebar` | `main-overlay` | `toolbar-right`。
- **默认聊天面**：消费 `markdown_text` / `message_text` / `summary_text` / `presentation.stream`；与 CLI 同语义（settle 全量 markdown、工具 summary 卡、dim 进度）。
- 命令输入：复用 commandPlane（`/help` `/lp` `/refresh` `/exit` 仅 CLI；Web 无进程退出，`/exit` 可提示用终端或关标签）。
- 事件桥：`window.LiteAgent`。

### 3. Panel 注入（插件业务 UI）

**Manifest：**

```json
{
  "ui": {
    "entry": "ui/index.html",
    "slots": ["sidebar"]
  }
}
```

- URL：`GET /plugin-ui/<name>/…` → 插件目录 `ui/`（path 清洗，禁止 `..` 逃逸）。
- 挂载时自动 `set` 到声明 slot；之后以运行时 `presentation.panel` 为准。

**帧形状（广播 evt，cap=presentation, method=panel）：**

```json
{
  "op": "set | append | clear",
  "slot": "sidebar",
  "id": "trace-list",
  "html": "<style>…</style><div>…</div>"
}
```

- `id` 必填；CSS 并入 `html` 字符串，Host 不解析。
- 同文档注入：`innerHTML` + 受信任 script；无 iframe/CSP。

### 4. UI Action

- Panel 内控件经 `LiteAgent.emitUIAction` → Host 发 `cap=ui, method=action` 到目标插件。
- Payload：`{"panel":"…","event":"…","value":…,"props":{…}}`。
- 无 `ui.action` handler：结构化 `method_not_found` + 前端 toast；**不**卸 Panel。
- 插件进程 down：保留最后 DOM；toolbar 标 unhealthy。

### 5. 事件通道

- `GET /events` SSE；`event: presentation|status|stream`；data 为现有 JSON。
- Replay：内存 ring buffer（默认 500）；`GET /events?replay=1` 先回放再 live。
- 不做跨进程持久化。

### 6. 前端 SDK（零 Node）

- Host Shell 注入全局：

```text
LiteAgent.call(cap, method, payload) -> Promise
LiteAgent.on(topic, fn)              // presentation | status | stream
LiteAgent.emitUIAction(panel, event, value, props?)
LiteAgent.markdown(text)             // 可选，复用壳 md
```

- 仓库提供可拷贝单文件 `sdk/lite-agent.js`（非 npm）。
- 插件 `ui/` 可手写 HTML/JS，或拷贝 petite-vue/alpine 单文件；Host 不打包。

### 7. CLI 共存

- 同进程：`-repl -serve` 时输入互斥（同一时刻至多一个 turn）。
- CLI 忽略 `panel`（未知 kind/method 安全丢弃，已有 warn 路径可复用或静默）。

## User Stories

1. 作为使用者，`host -serve` 后浏览器可对话，默认聊天面有 Markdown 与工具卡。
2. 作为插件作者，我在 `ui/index.html` 写面板，Manifest 声明 slot，无需 Node 构建。
3. 作为插件作者，运行时用 `EmitPanel` 更新轨迹列表；控件事件回到我的 `ui.action` handler。
4. 作为使用者，新开标签能看到最近历史（replay）再跟 live。
5. 作为开发者，CLI 与 Web 同时开，同一 Session 信号一致。

## Implementation Decisions

- **壳 + 默认聊天面归 Host**；业务 Panel 归插件——最小 demo 无需 UI 插件。
- **注入为主、信任作者**；不做沙箱（ADR-0009）。
- **SSE 复用 Presentation JSON**，不另造 web envelope。
- **固定三槽**，不做任意 slot 布局引擎。
- **protocol 2 扩展**，不 bump。
- **静态 entry + 运行时 panel** 双路径；CSS 打进 html 串。

## Testing Decisions

- 主缝：真实 Host + fixture；`httptest` 或本机端口拉 `-serve`。
- `/events` 能收到 settle markdown_text 与工具 summary_text。
- `presentation.panel` set/append/clear 改变 `/` 页 DOM（可用 goquery 或子串断言 HTML）。
- path 清洗：`/plugin-ui/x/../../etc` 拒绝。
- `ui.action` 无 handler → method_not_found。
- replay：先连后发/先发后连行为有测。
- CLI 回归不破。

## Out of Scope

- 多 Session 切换 UI、权限 modal（sandbox）、移动端、CSP/iframe。
- npm/前端构建链、React/Vue 工程化。
- 热插拔、跨进程 replay 持久化、独立 web 插件进程。

## Issue 分解（建议）

| # | 标题 | 依赖 |
|---|---|---|
| 01 | `-serve` + Shell HTML/JS + fan-out 到 SSE | — |
| 02 | 默认聊天面（stream/markdown/summary/message） | 01 |
| 03 | 命令输入接 commandPlane | 01 |
| 04 | `/plugin-ui/` 静态托管 + path 清洗 + Manifest ui | — |
| 05 | `presentation.panel` + 注入运行时 | 01,04 |
| 06 | `ui.action` 路由 + LiteAgent SDK | 05 |
| 07 | replay ring buffer | 01 |
| 08 | 示例插件（mode-switch / session-trace 占位） | 04–06 |
| 09 | README / 集成测 | 01–08 |

## Further Notes

- 术语以 CONTEXT.md（Panel / UI Action / Shell）与 ADR-0009 为准。
- CLI 路径零行为变更（除可选并存）。
- 作者文档：`sdk/lite-agent.js` 注释 + 示例 `ui/index.html`。
