# 2026-10-21 全链路压力测试报告

> 依据：[PRESSURE.md](../../PRESSURE.md)
> 被测版本：`e8611ce-dirty`
> 结论：主路径可用；发现 2 个 P0、6 个 P1、若干 P2。默认部署下编码任务被沙箱完全阻断；并行 subagent 会复用同一个子会话。

## 1. 范围

本轮执行 U1～U10、S1～S12、W1、W2，直接运行 `dist/` 产物，使用真实模型、真实工具与真实 Chrome。

S9 的 2 小时要求缩短为 5 分钟；S10 因缺少队列指标按阻塞处理，但已复现队列溢出。
S7 的 CDP 网络限速对已建立的 `/bridge/ui` 流不生效，改用页面冻结复现慢页，见 S7 说明。

## 2. 环境与数据

| 项 | 值 |
|---|---|
| 系统 | Windows 10.0.26200 x64 |
| Go | go1.25.6 windows/amd64 |
| Node | v24.19.0 |
| Python | 3.14.7 |
| Chrome | 155.0.8059.85，headless，视口 1440×900，CDP 9223 |
| 模型 | `mimo-v2.6-flash`（`https://token-plan-cn.xiaomimimo.com/v1`） |
| Host | `dist/liteagent-server.exe`，`127.0.0.1:2333` |
| 插件 | 9 个，全部 `active`；4 个 session mount |
| 数据 | D1 现有会话；新建 D2/D3 各 501 Facts；D4 50 会话、10 工作区、25,050 Facts、12.9 MB |

数据档位：

| 档位 | 内容 | 文件 |
|---|---|---|
| D2 | 250 条消息、501 Facts、1,259,755 B | `s-1792594695692521300.jsonl` |
| D3 | 250 条消息、501 Facts、1,684,758 B | `s-1792594695954352800.jsonl` |
| D4 | 50 会话、10 工作区、25,050 Facts、12,906,620 B | `.tmp-e2e/pressure-2026-10-21/d4-fixture.json` |

## 3. 静态与构建基线

| 检查 | 结果 |
|---|---|
| `go vet ./...` | 通过 |
| `go test ./... -count=1` | 13 个包中 12 个通过，`plugins/sandbox` 失败 |
| `node --test internal/webui/*.test.mjs` | 27 项全部通过 |
| `scripts/build.ps1 -Verify` | 失败并中止构建（被 sandbox 测试阻断） |
| `scripts/build.ps1 -Clean` | 通过，产出 `e8611ce-dirty` |
| Host 启动到 bootstrap 可读 | 579 ms（冷启动）；重启 309 ms |
| 首屏 FCP | 44～88 ms |

sandbox 失败项：

```text
--- FAIL: TestMatchPath/relative_pattern_absolute_path
--- FAIL: TestMatchPath/star_stays_in_one_scope_segment
--- FAIL: TestMatchPath/question_mark_matches_one_character
```

三项都表现为 `matchPath("README.md", "\\ws\\README.md") = false`。测试用反斜杠构造绝对路径，实现按 `/` 分段。Windows 下该测试无法通过。

构建陈旧问题：

- 首次增量构建判定 10 个目标全部 `unchanged`。
- 但 `dist/liteagent-server.exe` 启动日志版本是 `0d24ffb-dirty`，当前 HEAD 是 `e8611ce-dirty`。
- `-Clean` 重建后版本才变为 `e8611ce-dirty`。
- 增量缓存只校验源码哈希，不校验产物版本；产物被替换或缓存印记迁移时会静默复用旧二进制。

## 4. 问题清单

| ID | 级别 | 问题 | 状态 |
|---|---|---|---|
| I-01 | P0 | 默认沙箱拒绝 `file_write`/`shell_execute`，编码任务无法执行 | 已复现 |
| I-02 | P0 | 并行 subagent 共用同一个子会话 ID，上下文互相污染 | 已复现 |
| I-03 | P1 | 大会话首开和刷新不落在最新消息 | 已复现 |
| I-04 | P1 | 冻结/挂起标签恢复后不 resync，长期停在旧状态 | 已复现 |
| I-05 | P1 | 诊断面板在重新扫描后陈旧 | 已复现 |
| I-06 | P1 | `agent.config.get/set` 与设置页不暴露并发配置 | 已复现 |
| I-07 | P1 | 增量构建可能复用陈旧 dist 产物 | 已复现 |
| I-08 | P1 | Windows 下 `-Verify` 被 sandbox 测试失败阻断 | 已复现 |
| I-09 | P2 | 重新扫描被 10 秒挂载超时阻塞 | 已复现 |
| I-10 | P2 | 缺少队列指标，S10 无法按规程判定 | 已复现 |
| I-11 | P2 | `session.state` 随打开标签数线性变重 | 已复现 |
| I-12 | P2 | 小帧推送放大 3.4 倍，1 MiB 帧放大 2.0 倍 | 已复现 |
| I-13 | P2 | `session.append` 忽略文档中的 `timestamp` 字段 | 已确认 |
| I-14 | P2 | W1 模型提前收尾，9 Step 低于 15 Step 预期 | 已观察 |
| I-15 | P2 | W1 验证在 `test/` 下留下 `__pycache__` | 已观察 |
| I-16 | P2 | 诊断面板打开时抢走键盘焦点，违反平台契约 | 已复现 |

### I-01 P0 默认沙箱阻断写入与执行

默认 `dist/plugins/sandbox/plugin.json` 为 `autostart: true`，而 `plugins/sandbox/README.md` 写的是 `autostart: false`。
默认风险档为 50～79 `deny`、80～100 `deny`，`file_write`/`file_replace`/`file_patch` 风险 80，`shell_execute` 风险 90。

实测：

- `sandbox.check(file_write)` 返回 `deny`，原因 `risk 80 falls in band 80-100, action deny`。
- U3 中模型发起的 `shell_execute` 被拒，聊天区显示 `工具调用…失败`。
- W1 开始前必须临时加入四条 allow 规则，否则 `file_write`/`file_replace`/`file_patch`/`shell_execute` 全部不可用。

影响：开箱即用的“编码 Agent”只能读，不能写文件或运行命令。PRESSURE 的 W1/W2 与任何真实代码任务都被阻断。

建议：

- 二选一：恢复文档语义（`autostart: false`），或发布工作区范围的默认 allow 规则。
- 在确认流落地前，至少允许工作区内 `file_*` 写入与 `shell_execute`。
- 同步修正 README 与 manifest，避免部署语义不一致。

### I-02 P0 并行 subagent 共用子会话

构造：`tool_parallelism=auto`、`max_parallel_tools=4`，一条 assistant 消息内发出 3 个 `subagent` 调用。

实测三个 `subagent_spawned` 的 `child_session_id` 全部是 `s-1792596550961314300`，三次 `subagent_finished` 也指向同一 ID。
该子会话最终有 30 条 Fact，三个子代理的回答与上下文混在同一会话里；父会话的 `subagent` 栏只显示 1 个子代理。

原因：`plugins/session/session_ops.go` 自动生成 ID 使用 `time.Now().UnixNano()`，高并发下同一时钟刻度会得到相同值；`create` 又是幂等的，命中已存在 ID 时直接返回旧会话。
`plugins/subagent/runner.go` 未显式生成子会话 ID。

影响：子代理之间失去隔离，回答可能被其他任务污染；审计无法区分三个子会话。并发越高越容易触发。

建议：

- 自动 ID 改为 `crypto/rand` 或“时间戳 + 原子计数器”，并在创建锁内检查冲突。
- 自动 ID 冲突时重新生成；显式传入 ID 时才保留幂等语义。
- subagent 也可以自行生成唯一子 ID 并显式传给 `session.create`。

### I-03 P1 大会话不落到最新

D2（200 条窗口）首开：

- `scrollTop=26726`，`maxScroll=30923`，距底部 4197 px。
- 最后一条消息在视口下方 3559 px，`回到最新` 按钮可见。

D3 首开与刷新：

- 距底部 5659 px。
- 手动调用 `scrollToBottom()` 后 `distanceFromBottom=1`，最后一条消息可见。

原因：`scrollToBottom()` 在增量富文本布局完成前执行，后续渲染增加 `scrollHeight`，页面停留在旧底部。
相关代码：`plugins/session/ui/main.js:1951`、`main.js:2467`。

建议：在 `stickToBottom` 为真时用 `requestAnimationFrame` 双帧滚动，或对 `.messages` 挂 `ResizeObserver`，内容高度变化后继续保持贴底。

### I-04 P1 冻结标签不恢复

构造：4 个标签，第 4 个用 `Page.setWebLifecycleState(frozen)` 冻结；向选中会话追加 80 条消息；3 秒后恢复 `active`。

结果：

- 前 3 个标签均显示 80 条消息。
- 冻结标签恢复后 60 秒仍是 0 条，Host 徽标仍是“已连接”。
- 服务端没有 `queue full`，连接未被关闭，因此 SDK 不会触发重连与 `onResync`。

原因：SDK 只在重新收到 hello 时 resync（`internal/webui/static/lite-agent.js:154`）。连接没断时，被挂起期间丢失的推送没有任何补偿；帧尾部之后也没有新帧触发游标断点检测。

建议：

- 监听 `visibilitychange`/`pageshow`，页面恢复可见时主动重读 `session.state`/`bootstrap`。
- 或由 Host 周期性发送 heartbeat，SDK 检测游标停滞并 resync。
- 对 `document.visibilityState !== "visible"` 的连接降低广播或标注 stale。

### I-05 P1 诊断面板陈旧

U10：探针加入后服务端 `diagnostics=2`，面板仍为 0；移除后服务端 `0`，面板仍保留旧条目。
S11：移除探针后服务端 `0`，面板仍显示 3 条。

原因：`rescanPlugins()` 调用 `applyBootstrap()` 但不更新 `serverDiagnostics`，只有 `fetchBootstrap()` 会写该数组（`internal/webui/static/app.js:274`、`app.js:342`）。

建议：`rescanPlugins` 复用 `fetchBootstrap`，或把 `payload.diagnostics` 写回 `serverDiagnostics` 后再渲染。

### I-06 P1 并发配置不可通过 API 修改

`plugins/agent/types.go:161` 的 `configSetIn` 只有 `default_scheme` 和 `max_steps`；`configGetOut` 也只返回这两项。
`plugins/agent/README.md` 和 `config` 结构体却声明了 `tool_parallelism` 与 `max_parallel_tools`。

实测：

- `agent.config.set({tool_parallelism:"auto"})` 被静默忽略，`config.json` 保持 `serial`。
- 直接编辑 `config.json` 为 `auto` 并重启插件后，并发才生效。

建议：把两个字段补进 `configSetIn`/`configGetOut` 与设置 schema，并校验 `serial|auto` 与 1～16 范围。

### I-07 P1 增量构建复用陈旧产物

见第 3 节。建议在增量判断中加入产物版本校验：`go version -m` 或运行 `--version` 对比 `git describe`，不一致强制重建。

### I-08 P1 `-Verify` 在 Windows 失败

`plugins/sandbox/glob_test.go` 的 `TestMatchPath` 用反斜杠构造 Windows 绝对路径，实现按 `/` 分段，导致三个子测试失败。
`scripts/build.ps1 -Verify` 因此中止，无法产出经过验证的 dist。

建议：测试改用 `filepath.Join`，或在 `matchPath` 入口统一 `\` 与 `/`。

### I-09 P2 重新扫描被挂载超时阻塞

S11 点击“重新扫描”后，按钮被禁用 10,085 ms，期间面板状态停在“正在重新扫描…”。
原因是 `applyBootstrap({resync:true})` 等待 `mountLoader.resync`，而新挂载的 Custom Element 要等 10 秒超时才失败。

建议：重新扫描先完成插件列表更新并释放按钮，挂载对齐放到后台任务；超时诊断异步追加。

### I-10 P2 缺少队列指标

S10 复现：暂停一个 `/bridge/ui` 客户端，追加 120 条消息后服务端记录：

```text
ui: queue full id=ui-conn-118 closing=true
ui: connection closed id=ui-conn-118 total=2 drops=1
```

`/metrics`、`/bridge/metrics`、`/bridge/ui/metrics` 均 404，bootstrap 不含队列字段。
PRESSURE 要求“先补齐队列指标再测”，本轮只能给出溢出与丢帧证据，判为阻塞。

建议：在 bootstrap 或独立端点暴露每连接队列长度、入队/出队/丢弃计数、关闭次数。

### I-11 P2 `session.state` 随打开标签变重

U5 时 14 个打开标签，D2 的 `session.state` 响应 5,086,621 B，耗时 163～354 ms。
S9 5 分钟循环打开 50 个会话后，`state` p95 从 15 ms 升到 74 ms，最大 83 ms。

原因：`session.state` 返回当前 Workspace 全部打开会话的 ChatState，每个 pane 都带最多 200 条消息与 Trace 窗口。

建议：

- 只为选中 pane 返回完整消息；后台 pane 只返回摘要（id、标题、turn 状态、未读数）。
- 或按 pane 分页，页面激活时再拉取。
- 这与 [长会话性能设计](../design/webui-long-session-performance.md) 的窗口策略需要一起调整。

### I-12 P2 推送放大

S4 实测（25 次 append）：

- 4 KiB 消息：chat append 4.5 KB + trace append 4.5 KB + history State 5.5 KB，约 3.4 倍。
- 50 KiB 消息：chat 51.6 KB + trace 51.6 KB，约 2.0 倍。
- 1 MiB 消息：chat 1,048,954 B + trace 1,048,982 B，约 2.0 倍。
- 25 次 append 总推送 2,537,714 B；游标 0 断裂，无重复、无丢失。

建议：trace 推送只带 `seq/type/role/meta` 摘要，正文按需通过 `session.facts` 取；chat 与 trace 对同一 Fact 不重复传输正文。

### I-13 P2 `timestamp` 字段被忽略

`plugins/session/communication.md` 声明 `session.append` 输入支持 `timestamp`，但 `plugins/session/types_ops.go:29` 的 `factIn` 没有该字段。
批量导入的 Fact 全部使用写入时刻，D2/D3 的时间戳无法按数据构造。

建议：补字段并在缺省时回退当前时间，或修正文档。

### I-14 P2 W1 提前收尾

W1 完成 9 Step、11 次工具调用、3 个源文件，用时 298 秒，低于 PRESSURE 的 15 Step、约 10 分钟预期。
产物本身可用：13 个单元测试全部通过，`python -m py_compile` 通过，`test/` 外无改动。

建议：该差异来自模型决策，平台侧可在提示中要求“完成验证后给出文件清单与运行命令”，或在 Turn 结束前检查任务清单。

### I-15 P2 `__pycache__` 落入工作区

W1 的 Python 验证在 `test/__pycache__/` 生成 2 个 `.pyc`。
不影响功能，但污染产物目录。

建议：提示模型使用 `python -B` 或 `PYTHONDONTWRITEBYTECODE=1`，或在验收时清理。

### I-16 P2 诊断面板抢焦点

内置浏览器真实点击“诊断”后，面板出现，`document.activeElement` 变为 `#diagnostics`。
`setPanel()` 在打开时调用 `entry.panel.focus({preventScroll:true})`（`internal/webui/static/app.js:297`）。
平台契约要求诊断浮层“不抢占键盘焦点”（`docs/platform/webui.md` 第 9 节）。

影响：面板打开期间输入框失去焦点，用户继续输入会落空。视觉上面板非模态、无遮罩，Esc 可关闭并把焦点还给“诊断”按钮，这些行为正常。

建议：打开面板时不要抢焦点，保留触发按钮焦点；需要键盘关闭时把焦点放到关闭按钮，而不是面板容器。

## 5. 条目结果

### 5.1 可用性

| ID | 结果 | 实测 |
|---|---|---|
| U1 | 通过 | 真实鼠标点击新建 114 ms，点击命中 `create`；逐键输入 `ok` 成功；Enter 提交反馈 4 ms；1.35 s 完成 |
| U2 | 通过 | 2 个会话创建 121/108 ms；切换正确；搜索命中 1 条；刷新 359 ms；标签无重复 |
| U3 | 通过 | 真实点击停止 123 ms，终态 `cancelled`；工具阶段取消 110 ms；刷新后仍为取消 |
| U4 | 通过 | 视觉确认红色错误行、草稿保留；`error_code=llm_upstream`；重试 107 ms，7.1 s 完成 |
| U5 | 失败 | 真实滚轮 1200 px 后距底 1388 px，真实点击“回到最新”后距底 0；但首开仍距底 4197 px（I-03） |
| U6 | 通过 | 双页同步完成 1.28 s；Turn 中刷新 140 ms 未取消后端；两页终态一致 |
| U7 | 通过 | 默认模型改为 `gemma-4-e2b` 后新会话生效；已有会话保持 `mimo-v2.6-flash`；Key 仅掩码 |
| U8 | 通过 | 切换工作区 958 ms / 1074 ms；列表、pane、工作区一致；返回后恢复原选中会话 |
| U9 | 通过 | 失败后诊断实时出现；同键合并 `(3)`；稳定码 `llm_upstream`；刷新后仍在；视觉确认面板非模态、无遮罩、Esc 可关闭；打开时抢焦点（I-16） |
| U10 | 部分通过 | 服务端诊断正确、mount 不受影响、移除后清零；面板不刷新则陈旧（I-05） |

### 5.2 压力与错误

| ID | 结果 | 实测 |
|---|---|---|
| S1 | 通过 | 30 次 200 ms 刷新共 6.45 s；后端继续；恢复 462 ms；12 Facts 无重复；`state` 152～168 ms |
| S2 | 通过 | 工具阶段刷新 141 ms，Turn 继续 17.1 s 完成；流式阶段刷新 291 ms，取消 248 ms，终态 `cancelled` |
| S3 | 通过 | D3 首开 166 ms、刷新 285 ms；200 条窗口与选中状态保持；滚动问题同 I-03 |
| S4 | 通过 | 25 次 append；游标 0 断裂；1 MiB 帧约 2.0 倍；总推送 2.54 MB；页面 RTT 0～1 ms |
| S5 | 通过 | 1 MiB 工具输出；`已截断` 可见；展开前无输出节点，展开 1 ms；页面不冻结 |
| S6 | 通过 | D2 200 条窗口、501 Facts；加载更早 122 ms；锚点基本保持 |
| S7 | 失败 | 4 标签广播正常、无驱逐；冻结标签恢复后 60 s 仍 0 条，未 resync（I-04） |
| S8 | 通过 | 停止 agent 2.0 s；Session 补写 `turn_end(error)="agent plugin unavailable"`；Host 重启 309 ms，1.5 s 重连，状态一致 |
| S9 | 部分通过 | 5 分钟代替 2 小时；137 轮、11 次刷新、0 错误；`state` p95 74 ms；session 内存 38→48 MB |
| S10 | 阻塞 | 已复现 `queue full` 与 `drops=1`；无队列指标端点，按规程无法判定 |
| S11 | 通过 | 10 s 后 `ui_mount_failed`；模块已执行、元素未定义；其他 mount 正常；刷新不重复计数；面板陈旧同 I-05 |
| S12 | 通过 | 停 session 后启动 agent 返回 200 且 active；`start warn` 记录；发送得到 `method_not_found`；恢复 session 后成功 |
| W1 | 通过 | 9 Step、11 工具、3 文件、298 s；13 个单测通过；`test/` 外无改动；步数低于预期（I-14） |
| W2 | 失败 | 3 个 subagent 调用可并行（18.3 s），但共用 1 个子会话（I-02）；默认 `serial` 时 27.2 s |

S7 说明：CDP 的 `Network.emulateNetworkConditions` 对已建立的 NDJSON 流不生效，限速标签仍收到全部 40 条消息。改用页面冻结后复现了不恢复问题。

S8 补充：停止 agent 时，Session 在读取状态时补写 `turn_end(error)`；Host 重启后未出现残留 running，诊断清零。

### 5.3 真实输入与视觉补测

内置浏览器视口 1726×1191，截图逐张检查。它的指针/键盘工具无法进入 session 的 Shadow DOM：`browser_click` 即使给坐标也把事件派发到 slot 宿主，`browser_type` 拒绝非输入元素，`browser_press_key` 能触发文档级监听但不执行按钮默认动作。因此 Shadow DOM 内的鼠标与文本输入改用外部 Chrome + CDP `Input.dispatchMouseEvent`、`Input.dispatchKeyEvent`、`Input.insertText` 和 `mouseWheel` 复核，这条路径有真实命中测试与原生文本输入。

| 项 | 方式 | 实测 |
|---|---|---|
| U1 新建会话 | 真实鼠标点击 | 114 ms；`composedPath()[0]` 命中 `create`；新会话被选中 |
| U1 输入与提交 | 真实点击聚焦 + 逐键输入 + Enter | 点击后深层焦点为 `TEXTAREA`；逐键得到 `ok`；Ctrl+A 后插入中文提示；Enter 后 4 ms 进入 running，1.35 s 完成 |
| U3 停止 | 真实鼠标点击 | 123 ms 后终态 `cancelled`；部分正文保留，状态行显示“已取消” |
| U5 滚动 | 真实滚轮 + 真实点击 | 滚轮上移 1200 px 后距底从 188 px 变为 1388 px；点击“回到最新”后距底 0，按钮隐藏 |
| U4 失败态 | 视觉检查 | 红色错误行可见，输入框保留失败请求，诊断计数为 1 |
| U9 面板 | 内置浏览器真实点击 + Esc | 面板 420×176.7，位于右上，非模态、无遮罩；Esc 关闭并把焦点还给“诊断”按钮；打开时抢焦点（I-16） |

保真度边界：

- 已覆盖：真实命中测试、遮挡判断、鼠标按下/释放、逐键输入、原生文本插入、滚轮、按钮焦点与 Esc。
- 未覆盖：操作系统级 IME 组合输入、触摸/触控板惯性、悬浮提示、屏幕阅读器与完整 Tab 顺序。

## 6. 性能与资源实测

### 6.1 延迟

| 场景 | 实测 |
|---|---|
| Host 启动到 bootstrap | 579 ms（冷启动）、309 ms（重启） |
| 首屏 FCP | 44～88 ms |
| 新建会话反馈 | 108～121 ms |
| 发送反馈（进入 running） | 106～339 ms |
| 停止反馈 | 110～258 ms |
| 页面刷新恢复 | 140～989 ms |
| D2 `session.state` | 163～354 ms，5.09 MB |
| D4 `session.state` p95 | 74 ms（5 分钟末），最大 83 ms |
| 30 次刷新风暴恢复 | 462 ms |
| 加载更早消息 | 122 ms |
| 大输出展开 | 1 ms |

### 6.2 内存

| 进程 | 空闲 | W1 峰值 | S9 5 分钟 |
|---|---|---|---|
| liteagent-server | 14.8 MB | 19.5 MB | 40.3 MB |
| session | 10.6 MB | 14.1 MB | 58.1 MB |
| llm-openai | 7.8 MB | 21.6 MB | 21.4 MB |
| agent | 6.5 MB | 13.1 MB | 6.8 MB |
| 全部进程合计 | 约 81 MB | 123.3 MB | 173.6 MB |

浏览器 JS 堆：

- 空闲约 2.1 MB。
- 打开 D2/D3 与 14 个标签后约 19 MB。
- 展开 1 MiB 工具输出后增加 3.17 MB；GC 后可回落。

S9 期间 session 内存从 38.4 MB 升到 47.9 MB，未回落；5 分钟内无卡死。2 小时泄漏结论未取得。

### 6.3 IO、Facts 与 Token

| 项 | 实测 |
|---|---|
| S4 25 次 append 推送 | 2,537,714 B |
| 1 MiB append 推送 | chat 1,048,954 B + trace 1,048,982 B |
| D2 文件 | 1,302,849 B |
| D3 文件 | 1,684,758 B |
| D4 文件合计 | 12,906,620 B（25,050 Facts） |
| W1 会话 | 92 Facts、160,867 B |
| W1 Token | prompt 107,620 / completion 11,939 / total 119,559，9 次请求 |
| 会话文件总量 | 91 个、25.81 MB（含本轮测试数据） |
| Host 句柄峰值 | 412（S9） |

### 6.4 优化建议（按收益排序）

1. 降低 `session.state` 体积：后台 pane 只发摘要，选中 pane 才带正文与 Trace。当前 14 个标签时单次 5 MB，50 个会话时 p95 74 ms。
2. 消除 chat/trace 正文重复：trace 推 `seq` 与元数据，正文按需读取。可把 2.0～3.4 倍放大降到接近 1 倍。
3. 修复大窗口首开滚动：rAF 双帧或 `ResizeObserver` 贴底。
4. 页面可见性恢复时主动 resync，覆盖冻结/后台标签。
5. 诊断面板复用 bootstrap 快照，避免重新扫描后陈旧。
6. 暴露队列指标；队列满时关闭连接的行为已有，但缺少可观测性。
7. 约束 `session.state` 的并发重建：S9 中 `state` 耗时随打开会话数上升，可加缓存与失效标记，避免每次全量投影。
8. 增量构建增加产物版本校验，避免陈旧二进制。

## 7. 证据索引

证据目录：`.tmp-e2e/pressure-2026-10-21/`（已加入 `.gitignore`）。

| 路径 | 内容 |
|---|---|
| `build-verify.log`、`build-clean.log` | 静态检查失败与全量重建 |
| `server.err.log`、`server-restart.err.log` | 启动、插件生命周期、队列与错误日志 |
| `monitor.csv`、`monitor-s9.csv` | W1 与 S9 资源采样 |
| `smoke.json`、`u1.json`～`u10.json` | 可用性条目结果 |
| `u5-scroll.mjs`、`u5-scroll.png` | 大窗口滚动复现 |
| `s1.json`～`s12.json` | 压力条目结果 |
| `s7c.json`、`s7c-slow.png` | 冻结标签复现 |
| `w1.json`、`w1.png` | W1 产物与指标 |
| `w2.json`、`w2.png` | W2 并发结果；子会话 ID 见会话 Fact |
| `d4-fixture.json` | D4 数据清单 |
| `ux-real-input.json`、`ux-real-u1.png`、`ux-real-u3.png`、`ux-real-u5.png` | 真实鼠标、逐键输入、滚轮与停止按钮 |
| `ux-fail.png`、`ux-diagnostics.png` | 失败 Turn 视觉与诊断面板 |
| `ux_click_probe2.mjs` | 顶层与 Shadow DOM 真实点击命中对照 |
| `*.png` | 各条目页面快照 |

关键会话：

| 会话 | 用途 |
|---|---|
| `s-1792594695692521300` | D2 窗口边界 |
| `s-1792594695954352800` | D3 长会话 |
| `s-1792595924245288900` | W1 真实任务 |
| `s-1792596545430669600` | W2 并发 subagent（子会话冲突） |
| `s-1792596550961314300` | 被三个子代理共用的子会话 |

## 8. 未执行与偏差

| 项 | 状态 | 原因 |
|---|---|---|
| S9 2 小时 | 缩短为 5 分钟 | 本轮执行窗口不足；只记录 5 分钟实测 |
| S10 | 阻塞 | 缺少队列指标；已复现溢出与丢帧 |
| S7 真限速 | 替代 | CDP 限速不影响已建立 NDJSON 流；改用冻结页面复现 |
| W1 15+ Step | 未达到 | 模型 9 Step 收尾；产物与验证通过 |
| 内置浏览器 Shadow DOM 交互 | 工具受限 | 指针/键盘工具无法进入 shadow root；改用真实 CDP 输入补测 |
| 浏览器视觉回归 | 部分 | 关键页面已逐张检查；未做多分辨率与跨浏览器回归 |

## 9. 环境恢复

- LLM 配置：U4/U7 结束后配置哈希与备份一致，默认模型恢复为 `mimo-v2.6-flash`。
- agent 配置：`tool_parallelism` 恢复为 `serial`，`max_parallel_tools=4`。
- sandbox：4 条临时 allow 规则已删除，`rules.json` 已移除；`file_write` 重新返回 `deny`。
- 探针插件 `diag-probe`、`diag-slow` 已删除。
- `test/` 按 PRESSURE 要求保留；`__pycache__` 为验证副产物。
- 测试产生的会话与工作区保留在 `dist/plugins/session/sessions/` 与 `workspaces.json`。
- 测试结束后停止 Host 与 Chrome；插件无残留进程。

## 10. 复现

```powershell
# 静态
go vet ./...
go test ./... -count=1
node --test internal/webui/*.test.mjs

# 构建
powershell -File scripts/build.ps1 -Clean

# 启动
cd dist
.\liteagent-server.exe

# 浏览器测试
node .tmp-e2e\pressure-2026-10-21\u1.mjs
node .tmp-e2e\pressure-2026-10-21\u5.mjs
node .tmp-e2e\pressure-2026-10-21\s1.mjs
node .tmp-e2e\pressure-2026-10-21\w1.mjs
node .tmp-e2e\pressure-2026-10-21\w2.mjs
```
