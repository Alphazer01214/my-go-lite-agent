# my-go-lite-agent

轻量 **Go Agent 运行时**：Host 薄内核 + 进程外插件。拿到二进制和插件目录就能跑，不要求读源码；核心零第三方依赖。

```text
用户输入
   │
   ▼
┌──────────── Host（薄内核）────────────┐
│  Discovery · Assembly · 生命周期       │
│  Frame 路由（星型） · Session 不变量   │
│  默认 Agent Loop                      │
└───┬──────────┬──────────┬──────────┬──┘
    │          │          │          │
 session    llm     context-manager  tools
（插件）  （插件）    （插件）      （插件）
```

## 理念

- **一切皆插件**：Session、LLM、工具、上下文观测都是可发现、可组装、可替换的进程。
- **日志是真源**：会话事实只追加；模型能看到的内容必须能从 Session Log 重建。
- **轻**：进程隔离换崩溃边界与独立分发；不绑重框架，不堆臃肿 harness。

规范名词见 [CONTEXT.md](CONTEXT.md)；架构决策见 [docs/adr/](docs/adr/)。

## 特性

- 进程外插件（stdin/stdout JSON Frame），崩溃隔离
- Discovery ≠ Assembly：看见 ≠ 挂载
- 星型路由：插件不直连，策略平面唯一
- Session Log 不变量 + 默认 Agent Loop（可外置 `loop` 替换）
- Context Manager：System Prompt 组装、上下文占用、查看进入模型的 messages
- `llm-openai`：OpenAI 兼容（DeepSeek 等），流式输出
- CLI REPL / 一轮 `-turn`；Web Shell（聊天 + Session Trace）
- 跨平台：Windows / macOS / Linux

## 快速开始

### 构建

```powershell
# Windows
.\scripts\build.ps1
```

```bash
# macOS / Linux
bash scripts/build.sh
```

产物在 `dist/`：`liteagent-cli`、`liteagent-server`、`plugins/`、`examples/`。

### 配置模型

```powershell
# Windows（优先环境变量）
$env:OPENAI_API_KEY = "sk-..."
$env:OPENAI_BASE_URL = "https://api.deepseek.com/v1"   # 默认 DeepSeek
$env:OPENAI_MODEL = "deepseek-chat"
```

```bash
# macOS / Linux
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.deepseek.com/v1
export OPENAI_MODEL=deepseek-chat
```

也可编辑 `dist/plugins/llm-openai/config.json`，或在 REPL / Web 输入框里 `/llm-openai config set apiKey=sk-...`。

**构建不会动你的密钥**：`config.json` 不进 git；重新构建时，dist 里已有的配置原样保留（首次构建才会从仓库根的 `plugins/llm-openai/config.json` 播种）。

### 跑起来

```powershell
# 一轮对话
.\dist\liteagent-cli.exe -plugins dist\plugins -assembly dist\examples\chat.json -turn "你好"

# 多轮 REPL
.\dist\liteagent-cli.exe -plugins dist\plugins -assembly dist\examples\chat.json -repl

# Web
.\dist\liteagent-server.exe -plugins dist\plugins -assembly dist\examples\chat.json -serve 127.0.0.1:8080
# 浏览器打开 http://127.0.0.1:8080
```

```bash
# macOS / Linux
./dist/liteagent-cli -plugins dist/plugins -assembly dist/examples/chat.json -turn "你好"
./dist/liteagent-server -plugins dist/plugins -assembly dist/examples/chat.json -serve 127.0.0.1:8080
```

带文件工具：

```powershell
.\dist\liteagent-cli.exe -plugins dist\plugins -assembly dist\examples\agent.json -turn "读一下 README.md"
```

## 常用命令

**REPL / Web 输入框**

| 命令 | 说明 |
|------|------|
| `/help` | 帮助 |
| `/lp` | 已挂载插件 |
| `/session list` / `derive` / `dump-trace` | 会话列表 / Model Context / 导出日志 |
| `/context-manager usage` / `list` | 上下文占用 / 进入模型的 messages |
| `/llm-openai config` | 模型配置 |

原生 slash 仅 `/help` `/lp` `/refresh` `/exit`；其余能力在对应插件名下。

**CLI 一次成型**

```powershell
-turn TEXT           跑一轮
-context-list N      打印最近 prepare 的 N 条消息
-session-derive      打印 Model Context
-session-query       打印 Session Log 事实
```

## 装配示例

`dist/examples/chat.json`：

```json
{ "plugins": ["session", "llm-openai", "context-manager"] }
```

按需换成/追加 `filetools`、`echotool` 等；未点名的插件不会启动。

## 文档

- 术语与边界：[CONTEXT.md](CONTEXT.md)
- 架构决策：[docs/adr/](docs/adr/)
- 内部票与规格：[.scratch/](.scratch/)（开发用）
