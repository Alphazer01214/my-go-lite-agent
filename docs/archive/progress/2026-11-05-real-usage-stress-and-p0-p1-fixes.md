状态：仅参考

# 真实使用压力测试与 P0/P1 修复技术日志（2026-11-03 至 2026-11-05）

> 关联：[PRESSURE.md](../../PRESSURE.md)、[测试报告](../../testing/2026-11-03-real-usage-stress-report.md)
> 被测产物：初始 `dist/liteagent-server.exe` 为 `08c5552-dirty`，P1 修复后为 `31b5996-dirty`
> 本文记录本会话的测试、问题、根因、修改和验收。实现契约以平台文档与插件 README 为准。

## 1. 会话范围

本会话做了三件事：

1. 直接运行 `dist` 产物，用真实模型、真实工具和真实浏览器做使用与压力测试。
2. 修复两个 P0、取消终态和四个 P1，并加入会话选项卡。
3. 更新测试报告、平台文档和本技术日志。

测试与修复都在本机完成，没有改 Go 协议、Fact 结构和数据库。
唯一涉及后端代码的改动是 WebUI 诊断合并与 Mount 过滤。
其余修复集中在 session 插件的浏览器端。

## 2. 环境与基线

| 项 | 值 |
|---|---|
| 系统 | Windows 10.0.26200 x64 |
| Host | `dist/liteagent-server.exe`，`127.0.0.1:2333` |
| 初始版本 | `08c5552-dirty` |
| P1 后版本 | `31b5996-dirty` |
| 模型 | `mimo-v2.6-flash`，真实远端端点 |
| 浏览器 | Qoder 应用内浏览器，单页 |
| 工作区 | `E:\Projects\my-go-lite-agent` |
| 初始会话 | 9 个可见会话；dist 中共 23 个 JSONL |

数据档位：

| 档位 | 内容 |
|---|---|
| D0 | 新建空会话 |
| D1 | 现有会话，50 Facts 以内 |
| D2 | 250 条消息、500 Facts、约 1.2 MiB；后续追加到 525 Facts |
| D3 | D2 追加 1 MiB 单条事实，约 1.83 MB |
| D4 | 未执行 |

应用内浏览器只有单页。`window.open` 返回 `blocked`，U6 和 S7 的双标签场景无法真实执行。

## 3. 测试结果概览

初测结论：主链路可用，发现 2 个 P0、5 个 P1 和若干 P2。
P0 和 P1 在本会话全部修复，P2 保留。

| 类别 | 结果 |
|---|---|
| U1 首次对话 | 通过；端到端 82.6 秒，11 次工具调用，27 Facts |
| U2 会话管理 | 初测失败，已修复；刷新恢复标签和选中会话 |
| U3 停止 | 初测失败，已修复；取消终态可见 |
| U4 失败回合 | 初测失败，已修复；错误块和重试可见 |
| U5 长会话 | 初测部分通过，已修复；聊天与 Trace 窗口化 |
| U6 双标签 | 阻塞；应用内浏览器不能开第二页 |
| U7 模型设置 | 通过；默认模型生效，Key 不回显 |
| U8 工作区 | 通过；创建、切换、隔离和清理正常 |
| U9 诊断 | 初测部分通过，已修复；rescan 保留运行诊断 |
| U10 无效挂载 | 通过；探针只产生发现类诊断 |
| S1 快速刷新 | 30 次刷新后端稳定；刷新恢复在 P0 修复后验证 |
| S2 会话中刷新 | 通过；流式阶段刷新后回合继续，Fact 序号无重复 |
| S4 大帧 | 通过；1 MiB append 约 57 ms |
| S5 大工具输出 | 初测失败，已修复；默认截断，按需展开 |
| S8 Host 重启 | 通过；45 Facts 保留，页面自动重连 |
| W1 长程任务 | 通过；12 分 03 秒，14 次工具调用，4 个文件 |
| W2 并行 subagent | 通过；子会话 ID 不同，耗时 13.9 秒和 3.9 秒 |

## 4. 初测问题与根因

### 4.1 I-01 刷新丢失选中会话（P0）

现象：选中历史会话后刷新，列表仍在，主区回到“新会话”。

原因：

- `plugins/session/ui/store.js` 的 `bootstrap()` 只加载工作区、模型和会话列表。
- 没有读取或保存当前 `sessionId`。
- `selectSession()` 只修改内存状态。

### 4.2 I-02 失败回合刷新后消失（P0）

现象：模型端点不可用时，错误只在当前页面出现；刷新后只剩用户消息，也没有重试入口。

原因：

- `plugins/session/ui/chat.js` 只渲染 `message` 角色。
- `turn_end` 的 `status=error` 没有映射到聊天区。
- 失败分支只调用 `reportError()`，没有持久化错误块，也没有重试按钮。
- 错误同时出现在顶部 banner 和聊天区，重复展示。

### 4.3 I-03 取消终态不可见（P1）

现象：取消后聊天区显示“已停止 · 本轮内容未保存”，但助手消息已经写入 Facts；刷新后取消提示消失。

原因：

- “本轮内容未保存”是固定文案。
- 刷新后只从消息事实恢复，`turn_end.cancelled` 不参与渲染。

### 4.4 I-04 1 MiB 工具输出完整进入 DOM（P1）

现象：单条工具消息 1,048,583 字符；Trace 也把完整 preview 文本写入 DOM。

原因：

- 聊天工具消息直接创建完整文本节点。
- Trace 的 `factPreview()` 返回完整 `text_content`。
- `meta.truncated` 没有进入展示层。

### 4.5 I-05 rescan 清空运行时诊断（P1）

现象：触发 `loop.turn` 错误后诊断计数为 2；移除探针并 rescan 后，运行错误消失。

原因：

- `internal/webui/management.go` 在 rescan 时把 `s.diagnostics` 整体替换为 Registry 诊断。
- `PushDiagnostic` 追加的运行时诊断没有独立存储。

### 4.6 I-06 sandbox 未激活时产生常驻诊断（P1）

现象：Host 启动后诊断面板一直有 `sandbox.pending.list method_not_found`。

原因：

- Shell 对 Bootstrap 中的所有 Mount 都调用 Loader。
- sandbox 未激活，但它的 UI 已经挂载，`connectedCallback` 调用了不存在的 `pending.list`。
- 插件状态没有参与 Mount 决策。

### 4.7 I-07 长会话一次性渲染（P1）

现象：D2 会话一次渲染 250 条消息和 525 条 Facts，Trace 文本约 1.79 MB。

原因：

- 聊天 `renderMessages()` 遍历全部消息。
- Trace `render()` 遍历全部筛选后的 Facts。
- 没有窗口、分页或虚拟列表。

### 4.8 保留的 P2

| ID | 问题 | 状态 |
|---|---|---|
| I-08 | 运行期间标题、消息数、Facts 面板不更新 | 未修复 |
| I-09 | Host 日志记录完整帧且无轮转，约 30 分钟 3.28 MB | 未修复 |
| I-10 | LLM 流没有超时和 `max_tokens`，推理可持续数分钟 | 未修复 |
| I-11 | 错误文案保留原始传输细节，toast 与聊天区重复 | 部分缓解 |
| I-12 | `dist` 落后于 HEAD 和当前工作区 | P1 阶段已重建 Host |
| I-13 | shell 工具 command/args 契约容易被模型误用 | 未修复 |

## 5. 会话选项卡与 P0 修复

### 5.1 状态模型

在 `plugins/session/ui/store.js` 增加：

```text
tabs: string[]   // "" 表示新会话，其余是 session_id
active: string   // 当前标签，等同 state.sessionId
```

规则：

- 从历史列表选择会话时自动加入标签。
- 点击“新建会话”或标签栏加号时加入或复用 `""` 标签。
- 新会话收到 `started` 事件后，把 `""` 原位替换为 `session_id`。
- 关闭当前标签时优先切到右侧，其次左侧。
- 运行中的标签禁用关闭。
- 删除会话时同步删除标签。
- 标签按工作区保存到 `sessionStorage`。

存储键：`liteagent.session.tabs.v1`。

```json
{
  "E:\\Projects\\my-go-lite-agent": {
    "tabs": ["s-1793...", ""],
    "active": "s-1793..."
  }
}
```

选择 `sessionStorage` 的原因：

- 刷新可恢复。
- 每个浏览器页面独立，避免多页互相覆盖。
- 关闭浏览器后清除，符合“当前页面会话上下文”的语义。

### 5.2 标签栏 UI

`plugins/session/ui/chat.js` 在聊天区顶部加入标签栏：

- 激活标签使用 `--sx-active` 背景和 `--sx-accent-strong` 文字。
- 运行中标签显示脉冲圆点。
- 关闭按钮独立，运行中禁用。
- 加号新建会话。
- 标签过多时横向滚动。
- 激活标签变化时 `scrollIntoView`。

`plugins/session/ui/styles.js` 增加标签栏、关闭按钮、运行圆点和加载更早按钮样式。
颜色、圆角、字号沿用现有 `--sx-*` 变量。

### 5.3 P0 I-01 修复

启动流程增加 `restoreTabState()`：

1. 加载工作区和模型。
2. 加载会话列表。
3. 读取当前工作区的标签状态。
4. 过滤已删除的会话。
5. 恢复标签和激活会话。
6. 加载会话消息和 Facts。

`refreshSessions()` 增加 `pruneTabs()`，清理失效标签并处理当前会话被删除的情况。

### 5.4 P0 I-02 修复

聊天区不再只遍历消息，而是构建时间线：

- 按 `seq` 合并 `message` 和 `turn_end`。
- `error` 渲染“本轮执行失败”块。
- `cancelled` 渲染“本轮已取消 / 已保留部分输出”块。
- 错误块提供“查看详情”和“重试”。
- 重试调用现有 `sendTurn()`，重发该回合的用户输入。
- 重试不清空输入框草稿。
- 失败或取消后，等 Facts 落库再移除临时 Turn，避免重复显示。
- 错误块出现后不再额外弹“本轮”banner。

### 5.5 I-03 修复

- 取消文案改成“已停止”。
- 持久化 `turn_end.cancelled` 由时间线渲染。
- 刷新后仍显示取消终态和已保存的助手消息。

### 5.6 P0 验收

| 场景 | 结果 |
|---|---|
| 打开两个会话后刷新 | 两个标签和激活会话恢复 |
| 新建标签发首条消息 | 标签原位替换为真实标题，无重复 |
| 关闭非当前标签 | 当前会话不变 |
| 关闭当前标签 | 自动切到相邻标签 |
| 运行中标签 | 关闭禁用，标题为“生成中不可关闭” |
| 失败会话刷新 | 错误块和重试仍在 |
| 点击重试 | 重发原输入，草稿“草稿保留测试”未丢 |
| 取消会话刷新 | 显示“本轮已取消 / 已保留部分输出” |
| 跨工作区 | 标签隔离，切回后恢复 |
| 选项卡回归 | 3 个标签保持，控制台无错误 |

## 6. P1 修复

### 6.1 I-04 大输出截断与懒加载

聊天：

- 工具消息正文超过 8 KiB 时只写入前 8 KiB。
- 显示“已省略 N 字符，共 X MB”。
- “展开全文”按钮按需写入完整正文。
- 再次点击“收起”恢复截断态。

Trace：

- `previewText()` 将 preview 截断到 4 KiB。
- 截断时在首行显示“已截断：显示前 4,096 字符，共 N 字符”。
- 点击 fact 头仍可展开完整原始 JSON，这是用户主动操作。

验收：

| 指标 | 修复前 | 修复后 |
|---|---|---|
| 1 MiB 工具消息 DOM | 1,048,583 字符 | 8,236 字符 |
| 展开全文 | 无 | 1,048,610 字符 |
| 收起全文 | 无 | 8,236 字符 |
| Trace 最大单条预览 | 约 1 MiB | 4,128 字符 |
| 1 MiB 会话 Trace 文本 | 1,061,416 字符 | 17,297 字符 |

### 6.2 I-05 诊断合并

后端把诊断拆成两组：

```go
diagnostics []plugin.Diagnostic
discovery   []plugin.Diagnostic
runtime     []plugin.Diagnostic
```

规则：

- `NewServer` 把 Registry 诊断写入 `discovery`。
- `PushDiagnostic` 只追加到 `runtime`。
- `combineDiagnostics(discovery, runtime)` 合并两组并按 256 条截断。
- `rescan` 只替换 `discovery`，保留 `runtime`。

新增 `internal/webui/diagnostics_test.go`：

- 验证 rescan 后运行诊断仍在。
- 验证合并环上限和最新运行诊断保留。

验收：

| 场景 | 结果 |
|---|---|
| 运行错误后 rescan | `loop.turn handler_error` 保留 |
| 加入无效挂载探针 | 发现类 2 条 + 运行类 1 条 |
| 移除探针再 rescan | 发现类清零，运行类 1 条保留 |
| `go test ./internal/webui` | 通过 |

### 6.3 I-06 未激活插件不挂载 UI

Shell 在调用 Loader 前按插件状态过滤 Mount：

```js
const unavailable = new Set(
  plugins
    .filter((plugin) => ["inactive", "failed", "stopping"].includes(plugin?.state))
    .map((plugin) => plugin.name),
);
currentMounts = mounts.filter((mount) =>
  !unavailable.has(mount?.mount?.plugin ?? mount?.plugin));
```

注意：Bootstrap 的 mount 是嵌套结构 `{mount:{plugin,id}}`，过滤必须读 `mount.mount.plugin`。

行为：

- 未激活插件不加载 UI。
- 插件启动后，Bootstrap 对齐会挂载 Mount。
- 插件停止后，下一次对齐会移除 Mount。

验收：

| 场景 | 结果 |
|---|---|
| 干净启动 | `sandbox-confirm` 不挂载，诊断 0 |
| 启动 sandbox | Mount 出现，`pending.list` 返回 `pending: []`，诊断 0 |
| 停止 sandbox | Mount 移除，诊断 0 |

### 6.4 I-07 聊天与 Trace 窗口化

聊天：

- 默认渲染最近 80 个时间线条目。
- 更早条目通过“加载更早的 N 条”按钮展开。
- 每次增加 80 条。
- 加载更早时补偿 `scrollHeight` 差值，保持当前阅读位置。

Trace：

- 默认渲染最近 200 条 Facts。
- 每次加载更早增加 200 条。
- 同样补偿滚动位置。
- 切换会话或筛选时重置窗口。

验收：

| 指标 | 修复前 | 修复后 |
|---|---|---|
| D2 聊天消息 DOM | 250 | 80 |
| D2 聊天文本 | 644,292 字符 | 216,698 字符 |
| D2 Facts DOM | 525 | 200 |
| D2 Trace 文本 | 约 1.79 MB | 47,335 字符 |
| 加载更早聊天 | 无 | 80 → 160 → 240 |
| 加载更早 Facts | 无 | 200 → 400 |

窗口化只减少默认 DOM 数量，不是完整虚拟列表。

## 7. 构建与验证

### 7.1 构建

| 阶段 | 命令 | 结果 |
|---|---|---|
| P0 | `powershell -File scripts/build.ps1 -Only session` | 更新 session UI |
| P1 | `powershell -File scripts/build.ps1 -Only liteagent-server,session` | 重建 Host 和 session 资源，版本 `31b5996-dirty` |

`dist` 中其他插件二进制仍来自旧构建。正式发布前需要完整重建。

### 7.2 自动化检查

| 检查 | 结果 |
|---|---|
| `go test ./internal/webui -count=1` | 通过 |
| `node --check` 修改的 JS | 通过 |
| 浏览器控制台 | 无错误 |
| P1 验证网络请求 | 均返回 200 |

### 7.3 真实浏览器验收

- 选项卡：创建、切换、关闭、新会话替换、跨工作区恢复。
- P0：刷新恢复选中会话；失败块和重试；取消终态。
- P1：1 MiB 工具消息截断与展开；D2 聊天、Facts 窗口与加载更早；rescan 诊断保留；未激活插件 Mount 过滤。
- W1：生成 4 个 Python 文件，12 个单测通过，SMOKE PASS。
- W2：两个 subagent 子会话 ID 不同，回答无污染。

W1 总耗时 722,568 ms，41 Facts，26 条消息，推理文本 62,843 字符。
W2 子代理耗时 13,941 ms 和 3,881 ms。
Host 峰值内存约 31.9 MB，句柄数约 406，快速刷新后未持续上涨。

## 8. 修改文件

| 文件 | 改动 |
|---|---|
| `plugins/session/ui/store.js` | 标签状态、sessionStorage、恢复、关闭、失效清理、终态清理 |
| `plugins/session/ui/chat.js` | 标签栏、时间线、错误/取消块、重试、大输出截断、聊天窗口 |
| `plugins/session/ui/styles.js` | 标签栏、运行圆点、错误块、展开与加载更早样式 |
| `plugins/session/ui/trace.js` | 预览截断、Facts 窗口、加载更早 |
| `plugins/session/README.md` | 选项卡、终态、大输出、窗口化说明 |
| `internal/webui/server.go` | discovery/runtime 诊断分离与合并 |
| `internal/webui/publisher.go` | 运行诊断环形缓冲与合并 |
| `internal/webui/management.go` | rescan 保留运行诊断 |
| `internal/webui/static/app.js` | 按插件状态过滤 Mount |
| `internal/webui/diagnostics_test.go` | 诊断合并测试 |
| `docs/platform/webui.md` | Mount 过滤与诊断合并语义 |
| `docs/platform/internal_webui.md` | 诊断保留、rescan、Loader 过滤 |
| `docs/testing/2026-11-03-real-usage-stress-report.md` | 测试结果与 P0/P1 修复记录 |

## 9. 决策与取舍

| 决策 | 原因 |
|---|---|
| 标签使用 `sessionStorage` | 刷新恢复，页面隔离，关闭浏览器清除 |
| 最多一个“新会话”标签 | 降低状态复杂度，重复点击复用 |
| 运行中标签禁止关闭 | 避免用户失去唯一的运行视图 |
| 失败重试直接重发原输入 | 满足“重试不丢草稿”，不在输入框填充 |
| 取消终态与错误共用时间线 | 同一套 `turn_end` 数据，避免两套渲染 |
| 窗口化而非完整虚拟列表 | 先解决 D2/D3 全量 DOM 问题，降低改动风险 |
| 大输出展开仍一次性写入 | 用户主动操作，优先实现简单可靠 |

## 10. 遗留与边界

- P2 I-08 运行期元数据不更新，仍在。
- P2 I-09 日志无轮转，仍在。
- P2 I-10 LLM 流无超时和 `max_tokens`，仍在。
- I-11 错误文案仍有传输细节，本轮只消除了重复 banner。
- I-13 shell 工具契约仍需优化。
- D4 档位和 2 小时长跑未复测。
- 窗口化不是完整虚拟列表；展开 1 MiB 输出仍会一次性写入。
- 刷新后不重新订阅修复前已经运行的 Turn。
- `dist` 只重建了 Host 与 session，其他插件仍是旧二进制。
- U6/S7 双标签因应用内浏览器限制未执行。

## 11. 环境副作用

| 项 | 状态 |
|---|---|
| 测试 Host | 已停止 |
| 临时探针插件 | 已删除 |
| 临时工作区 | 已删除 |
| 默认模型 | 已恢复 `mimo-v2.6-flash` |
| llm-openai Key | 保存后仍存在，未清空 |
| W1 `test/` | 保留 4 个文件；已被 `.gitignore` 忽略 |
| 测试会话 | 保留在 `dist/plugins/session/sessions`，供复查 |
| 测试日志 | 保留在 `dist/server-20261103-*.log` |
| 浏览器页面 | 已重新加载，无控制台错误 |

## 12. 文件索引

| 位置 | 关系 |
|---|---|
| `plugins/session/ui/store.js` | 标签与选中状态、终态清理 |
| `plugins/session/ui/chat.js` | 聊天时间线、选项卡、大输出、窗口化 |
| `plugins/session/ui/trace.js` | Facts 预览与窗口化 |
| `plugins/session/ui/styles.js` | 新增 UI 样式 |
| `internal/webui/static/app.js` | Mount 状态过滤 |
| `internal/webui/server.go` | 诊断合并 |
| `internal/webui/publisher.go` | 运行诊断环形缓冲 |
| `internal/webui/management.go` | rescan 语义 |
| `internal/webui/diagnostics_test.go` | 诊断测试 |
| `docs/testing/2026-11-03-real-usage-stress-report.md` | 完整测试与修复记录 |
| `docs/platform/webui.md` | 平台展示契约 |
| `docs/platform/internal_webui.md` | Host 内部实现说明 |
| `plugins/session/README.md` | Session 插件行为说明 |
