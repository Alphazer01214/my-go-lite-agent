# cmd/liteagent-server — Web 入口

## 职责

Web Render Medium 的进程入口。`main.go` 调用 `app.Server()`；装配、Layout 合并、Host 启动与 HTTP 面在 [internal/app](internal-app.md) + [web](web.md)。

## 结构

```text
cmd/liteagent-server/
  main.go           # package main + app.Server()
  *_test.go         # 端到端二进制测试
  sessions/         # 测试会话数据
```

## 入口 flag

| Flag | 说明 |
|------|------|
| `-plugins` | 必填；插件根目录 |
| `-serve` | 必填；监听地址，如 `127.0.0.1:7788` |
| `-layout` | 可选；Layout 路径，默认 `layout.json` |
| `-repl` | 可选；同进程再开 CLI REPL（共享同一 `serve.Server`） |
| `-dump` / `-debug` | 诊断 |
| `-assembly` | **已废弃**（ADR-0021） |

## 启动链

见 [internal/app](internal-app.md) 的 `runWebAndOptionalREPL`：Discovery → Autostart 闭包 → Layout Merge → `serve.Start` → workspace 种子 → `web.New` → listen。

## 测试

`web_test.go`：以真实二进制起服，覆盖

- Shell 可访问
- `/api/call` 发起 turn + SSE 观测
- `/api/command` → `/help`
- turn 后 `session.query` 历史

## 开发约定

1. 保持薄 main；HTTP/领域逻辑不进 `cmd/`。
2. 启动选项保持 L0（无 session/agent/scheme 专用 argv）。
3. 端到端测试优先改本目录套件。
