# 最小组合运行交接

> 状态：已废止
> 说明：最小组合的冻结状态描述。三个 Mount 的预期已过时，当前是四个。

日期：2026-10-15  
分支：`webui`  
对象：`agent + llm-openai + session`  
状态：应用已停止，运行现场已保留。

## 1. 停止状态

原运行地址为 `http://127.0.0.1:2333/`，PID 为 `53480`。该进程已终止。`minimal-agent-stack-monitor` 已删除。

## 2. 保留现场

```text
dist/minimal/liteagent-server.exe
dist/minimal/plugins/agent/
dist/minimal/plugins/llm-openai/
dist/minimal/plugins/session/
dist/minimal/server.out.log
dist/minimal/server.err.log
dist/minimal/plugins/session/sessions/
```

`dist/minimal/plugins/llm-openai/config.json` 含本机 LLM 配置，不提交、不复制、不写入本文。仓库源码配置不含真实密钥。

## 3. 当前实现

`agent`：

- 创建 `turn_id` 和 `step_id`；
- 写入 Session Fact 元数据；
- 累积 `llm.chunk`；
- 合并快速 Stream Delta；
- 调用 `session.stream.update`；
- 取消时保存累计 assistant 正文；
- Turn 开始时固定配置快照。

Stream 发布策略：

```text
首个 Delta 立即发布
最多每 24 个 Delta 发布一次
至少间隔 100 ms
complete 和 cancelled 始终发布
```

`llm-openai`：

- OpenAI-compatible Completion；
- 支持 Reasoning、Content、Tool Call Delta；
- 每次 Call 使用开始时配置；
- 配置修改影响后续 Call；
- `config.json` 使用 `0600` 权限保存。

`session`：

- JSONL 是 Session Fact 真源；
- 支持 `session.state` 重建 UI State；
- 支持 `session.stream.update`；
- 最终 assistant message 清除 Stream Draft；
- 提供 `session-history`、`session-chat`、`session-trace`。

插件实现只依赖 `pluginsdk`，没有导入 `internal/host`。

## 4. 已修复问题

详细记录见 [最小组合调试日志](2026-10-15-minimal-stack-debug-log.md)。

1. Stream Snapshot 和 UI Command 过量；
2. Stream Draft 被普通 ChatState 提前清除；
3. LLM 配置文件权限过宽；
4. 最小配置调用不存在的 tools Capability，产生预期错误噪音。

## 5. 验证结果

通过：

```text
go test ./...
go test -race ./plugins/agent ./plugins/session ./plugins/llm-openai ./pluginsdk
go vet ./plugins/agent ./plugins/session ./plugins/llm-openai
node --check plugins/session/ui/main.js
git diff --check
```

真实测试结果：

- 短 LLM Turn 返回正确内容；
- 浏览器 Chat 发送和 State 恢复正常；
- 浏览器控制台无 error 或 warning；
- 取消时保存部分 assistant 正文；
- 多 Session 并发不串数据；
- 同 Session 并发 Turn 正确排队；
- Bootstrap 诊断为 0；
- 未出现 `plugin_down`、`context_closed`、`llm_upstream` 或 panic。

## 6. 重启入口

运行目录：

```text
E:\Projects\my-go-lite-agent\dist\minimal
```

启动程序：

```text
liteagent-server.exe
```

环境变量：

```text
LITEAGENT_HTTP_ADDR=127.0.0.1:2333
```

启动后检查 `GET /bridge/bootstrap`。预期插件为 `agent,llm-openai,session`，Mount 为 `history,chat,trace`，诊断数为 0。

## 7. 重新构建

```text
go build -o dist\minimal\plugins\agent\agent.exe ./plugins/agent
go build -o dist\minimal\plugins\llm-openai\llm-openai.exe ./plugins/llm-openai
go build -o dist\minimal\plugins\session\session.exe ./plugins/session
go build -o dist\minimal\liteagent-server.exe ./cmd/liteagent-server
```

同步 `plugins/session/plugin.json`、`config.json`、`README.md` 和 `ui/main.js`。重建前保留 LLM 配置和 Session 数据。

## 8. 下一步

1. 改善 Markdown 正文显示；
2. 将 Trace 面板改为虚拟列表或分页；
3. 合并三个 Component 的 `session.state` 读取；
4. 为上游 LLM Call 增加显式取消或超时；
5. 根据真实日志决定是否增加资源配额；
6. 再考虑 tools、memory 和多 LLM Instance。

## 9. 工作区

Git 工作区仍有大量未提交改动。交接前未执行 commit、push 或清理。

主要文档：

```text
docs/archive/progress/2026-10-15-host-runtime-infrastructure-development-log.md
docs/archive/progress/2026-10-15-agent-llm-session-minimal-development-log.md
docs/archive/progress/2026-10-15-minimal-stack-debug-log.md
docs/archive/progress/2026-10-15-minimal-stack-handoff.md
```
