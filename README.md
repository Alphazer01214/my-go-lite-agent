# LiteAgent

LiteAgent 是一个本地优先的 Go Agent 平台。
Host 只负责插件进程、Frame 路由和 WebUI 平台壳，业务能力由进程外插件提供。

## 快速开始

要求 Go 1.25.6 或更高版本。使用 Git 面板时还需要本机安装 Git。

```bash
bash scripts/build.sh
./dist/liteagent-server
```

Windows：

```powershell
powershell -File scripts\build.ps1
.\dist\liteagent-server.exe
```

启动后打开 `http://127.0.0.1:2333`。
监听地址可用 `LITEAGENT_HTTP_ADDR` 覆盖。

首次运行前，按需把 `plugins/<name>/config.example.json` 复制为
`plugins/<name>/config.json`。真实密钥只保存在本地，`config.json` 不会进入 Git。

## 插件

| 插件 | 作用 |
|---|---|
| `agent` | Turn 循环、工具调用和控制 |
| `session` | 工作区、会话事实和会话 UI |
| `llm-openai` | OpenAI 兼容模型 Provider |
| `prompter` | Prompt 装配、Skills 和 MCP |
| `tools` | 工具目录和分发 |
| `file-tools`、`shell-tools`、`python` | 具体工具 |
| `subagent` | 子会话委派 |
| `git-tool` | 分支状态和 diff 面板 |
| `sandbox` | 可选调用策略 |

## 目录

| 目录 | 内容 |
|---|---|
| `cmd/liteagent-server` | Host 入口 |
| `internal/host`、`internal/webui`、`internal/plugin` | Host 平台 |
| `pluginsdk`、`protocol` | 插件 SDK 与 Frame 协议 |
| `plugins/` | 插件及其 WebUI |
| `docs/` | 平台、插件和设计文档 |
| `scripts/` | 构建脚本和目标清单 |

## 文档

- [AGENT.md](AGENT.md)：根目录文档导读
- [ARCHITECTURE.md](ARCHITECTURE.md)：平台结构
- [CONTEXT.md](CONTEXT.md)：平台术语
- [CONSTRAINT.md](CONSTRAINT.md)：协作与测试规则
- [docs/platform/](docs/platform/)：平台契约
- `plugins/<name>/README.md`：插件契约

## 测试

```bash
go test ./...
```

可选安装 Node.js 后运行 UI 测试：

```bash
node --test plugins/session/session-list.test.mjs
```

## 范围

v1 面向本机浏览器，不提供多用户、远程访问、跨设备同步或插件市场。
插件 JS 按本机可信代码处理。
