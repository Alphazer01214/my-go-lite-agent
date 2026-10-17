# 最小组合调试日志

日期：2026-10-15  
分支：`webui`  
对象：`agent + llm-openai + session`  
运行地址：`http://127.0.0.1:2333/`  
状态：已定位并修复 3 个问题。当前进程持续运行。

## 1. 测试范围

本次覆盖：

1. 单元测试和 race 测试；
2. HTTP Bootstrap 和插件注册；
3. 浏览器 UI 加载与控制台；
4. Session State 刷新；
5. 真实 LLM Turn；
6. 流式输出；
7. Turn 取消；
8. 多 Session 并发；
9. 同 Session 并发排队；
10. 配置文件权限。

## 2. BUG-001：流式更新洪水

### 现象

长回答产生大量重复 Stream Snapshot。

修复前采样：

```text
stream.update       2871 次
ui.command(stream)  2871 次
Host Frame 行        23336 行
```

浏览器没有报错，但 Host 日志和 UI 推送量随 token 数线性增长。

### 定位

Agent 把每个 `llm.chunk` 都转换成一次 `session.stream.update`。

Session 又为每次更新调用一次 `host.ui.command`。因此一次 LLM Delta 会放大为多次跨插件和 WebUI Frame。

### 修复

Agent 新增 Stream Flush Policy：

```text
首个 Delta 立即发布
同一 Step 最多每 24 个 Delta 发布一次
同一 Step 至少间隔 100 ms 发布一次
最终 complete 始终发布
取消时始终发布 cancelled 快照
```

快速 Delta 会合并，慢速 Delta 仍能及时显示。

### 验证

新增测试：

- `TestStreamFlushPolicyCoalescesFastDeltas`
- `TestStreamFlushPolicyKeepsSlowDeltasVisible`

真实短 Turn 结果：

```text
stream.update       3 次
ui.command(stream)  3 次
```

长回答和取消测试中，更新次数保持有界。

## 3. BUG-002：Stream Draft 被普通 State 提前清除

### 现象

运行中的 Stream Draft 可能被 `ChatState` 更新提前清空。

ChatState 的 Turn 状态使用：

```text
running
completed
cancelled
max_steps
```

前端却用 `status !== "streaming"` 判断是否清除 Draft。`running` 也会命中该条件。

### 修复

只在终态清除 Draft：

```text
completed
cancelled
max_steps
error
```

运行中的 `ChatState` 不再删除临时正文。

### 验证

浏览器重新加载后：

- Session History 正常；
- ChatState 可恢复；
- 取消后的部分 assistant 正文保留；
- 控制台无 error 或 warning。

## 4. BUG-003：LLM 配置文件权限过宽

### 现象

llm-openai 使用 `0644` 写入 `config.json`。该文件包含 API Key。

### 风险

在支持 POSIX 权限的系统上，同机其他用户可能读取密钥。

### 修复

写入权限改为：

```text
0600
```

写入后显式执行 `chmod 0600`。Windows 上行为由系统 ACL 决定。

## 5. 最小配置噪音

最小组合没有 tools 插件，但 Agent 默认使用 `tool_calling`。每个 Turn 都会调用 `tools.list` 并产生：

```text
method_not_found
```

行为正确，但日志存在预期噪音。

最小运行目录已改用：

```text
default_scheme = chat
```

修复后：

```text
tools method_not_found = 0
```

以后加载 tools 插件时，可在 Settings 中切回 `tool_calling`。

## 6. 功能验证

### 真实 LLM

短指令：

```text
Reply with exactly FINAL-OK.
```

结果：

```text
error_code = 空
reply = FINAL-OK
steps = 1
```

### 浏览器 UI

在 Session Chat 输入：

```text
Reply with exactly UI-OK.
```

结果显示：

```text
UI-OK
```

浏览器控制台无 error 或 warning。

### 取消

长回答流式输出期间调用 `loop.cancel`：

```text
cancel_result = true
turn_cancelled = true
turn_reply_chars = 717
assistant_count = 1
turn_end.reason = cancelled
```

取消前已累计的 assistant 正文和 reasoning 均已保存。

### 多 Session 隔离

并发执行：

```text
Session A：Reply with exactly A.
Session B：Reply with exactly B.
```

结果：

```text
A.session assistant = A
B.session assistant = B
```

没有串 Session。

### 同 Session 排队

同一 Session 并发执行两个 Turn：

```text
replies = S1, S2
turn_starts = 2
unique_turn_ids = 2
```

Turn 没有交错写入同一活动槽。

## 7. 当前运行状态

```text
PID = 53480
diagnostics = 0
plugins = agent, llm-openai, session
mounts = history, chat, trace
plugin_down = 0
context_closed = 0
llm_upstream = 0
panic = 0
```

持续监控仍由 `minimal-agent-stack-monitor` 负责。

## 8. 回归命令

```text
go test ./...
go test -race ./plugins/agent ./plugins/session ./plugins/llm-openai ./pluginsdk
go vet ./plugins/agent ./plugins/session ./plugins/llm-openai
node --check plugins/session/ui/main.js
git diff --check
```

全部通过。

## 9. 遗留事项

本次未处理：

- Markdown 正文仍按纯文本显示；
- Trace 面板仍展示完整 JSONL；
- Session State 由三个 Component 分别读取；
- 上游 LLM Call 没有显式取消协议；
- Session 和工具调用暂无资源配额。

这些项目尚无确定故障信号。后续根据真实使用日志再决定是否修改。
