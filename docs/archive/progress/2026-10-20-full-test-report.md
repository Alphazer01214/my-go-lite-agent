# 2026-10-20 全量测试报告

> 状态：仅参考
> 说明：U1 到 U8、S1 到 S8、W1 的执行证据与 I-01 到 I-10 的问题清单。后续修复见同目录的 fix 记录。问题清单尚未全部关闭。

## 1. 测试范围

用户在 2026-10-20 明确要求全面测试。

本次执行 U1～U8、S1～S8 与 W1。
S9、S10 未执行，原因见第 6 节。

测试直接运行 `dist/` 产物。
使用真实模型、真实工具、真实浏览器。
未复制目录，未使用假 LLM 或假 UI。

### 1.1 环境

| 项 | 值 |
|---|---|
| 系统 | Windows 10.0.26200 x64 |
| Go | go1.25.6 windows/amd64 |
| 版本 | `93f0e45-dirty` |
| Host | `dist/liteagent-server.exe` |
| 浏览器 | 内置浏览器；无头 Chrome 155.0.8059.85 |
| 模型 | `mimo-v2.6-flash` |
| 数据 | 现有会话；新建 D2 与临时测试会话 |

`dist/` 构建时间早于部分源码修改。
测试前运行 `scripts/build.ps1 -Verify`。
脚本判定 8 个目标哈希未变，保留 11 个会话文件。
构建版本为 `93f0e45-dirty`。

### 1.2 静态基线

| 检查 | 结果 |
|---|---|
| `go vet ./...` | 通过，无输出 |
| `go test ./... -count=1` | 13 个包全部通过 |
| `node --test internal/webui/*.test.mjs` | 25 项全部通过 |
| `scripts/build.ps1 -Verify` | 通过 |

Host 启动后 7 个插件均为 `active`。
启动诊断为空。

## 2. 结论

主路径可用：对话、取消、标签恢复、双页广播、长会话分页。
本次发现 1 个 P0 问题。
另发现 4 个 P1 问题与若干 P2 观察项。

最严重的问题是快速刷新叠加流式输出。
它会让 session 插件停止响应。
页面随后变空，Turn 一起停住。
只能通过重启插件或 Host 恢复。

### 2.1 问题清单

| ID | 级别 | 问题 | 状态 |
|---|---|---|---|
| I-01 | P0 | 快速刷新叠加流式输出会卡死 session 插件 | 已复现 |
| I-02 | P1 | 失败 Turn 刷新后不可见，错误文案为内部术语 | 已复现 |
| I-03 | P1 | 设置页修改默认模型后，新会话仍用旧模型 | 已复现 |
| I-04 | P1 | 切换工作区后仍选中旧工作区会话 | 已复现 |
| I-05 | P1 | 插件重启后页面仍显示“生成中”，需手动刷新 | 已复现 |
| I-06 | P2 | 取消 Turn 无标记，旧请求进入下一轮上下文 | 已复现 |
| I-07 | P2 | 慢连接丢帧后可能长期停在旧状态 | 已复现 |
| I-08 | P2 | 工具错误暴露底层错误文本 | 已观察到 |
| I-09 | P2 | W1 模型提前收尾，未完成说明与验证 | 已观察到 |
| I-10 | P2 | Host 重启后残留 `network error`，无 Turn 折叠 | 已复现 |

### 2.2 后续修复状态

2026-10-20 已实施 P0 Phase 1、Phase 2 与 P1。

| 问题 | 状态 | 文档 |
|---|---|---|
| I-01 P0 死锁 | 已修复 | [P0 写锁修复](2026-10-20-p0-write-lock-fix.md) |
| Phase 2 流式 outbox | 已实施 | [Phase 2 与 P1](2026-10-20-phase2-p1-fix.md) |
| I-02 失败 Turn 不可见 | 已修复 | 同上 |
| I-03 默认模型不生效 | 已修复 | 同上 |
| I-04 工作区不一致 | 已修复，行为有变更 | 同上 |
| I-05 插件重启 stale | 已修复 | 同上 |
| I-06 取消语义 | 已修复 | [P2 修复](2026-10-20-p2-fix.md)：记录取消提示 Fact，聊天区过滤 |
| I-07 慢连接丢帧 | 已修复 | 同上：队列溢出关闭连接，SDK 重连 resync |
| I-08 工具错误文案 | 已修复 | 同上：包含动作与路径 |
| I-09 W1 提前收尾 | 未修复 | 模型行为 |
| I-10 重启错误残留 | 已修复 | resync/refresh 成功后清理 |

## 3. 问题详情

### I-01 快速刷新叠加流式输出会卡死 session 插件

级别：P0。

状态：已修复 Phase 1。
实现与实测见 [P0 写锁与超时修复](2026-10-20-p0-write-lock-fix.md)。

现象：

1. W1 Turn 正在执行。
2. 以 200 ms 间隔刷新页面 30 次。
3. 最后一次刷新后，页面 20 秒内没有恢复。
4. 页面显示“尚未添加”和空会话列表。
5. `session.workspace.list` 只返回响应头，body 不结束。
6. W1 的 JSONL 停在 36 条 Fact，没有 `turn_end`。
7. `bootstrap` 和 `llm.models.list` 仍能正常响应。
8. 只有 `session` 能力不可用。

复现路径：

- 主会话：`s-1792483037139265900`。
- 刷新工具：`.tmp-e2e/live/cdp-refresh.mjs`。
- 参数：30 次，200 ms。

证据：

- `.tmp-e2e/live/after-refresh.png`。
- 截图显示空工作区与空历史。
- `.tmp-e2e/live/server-2026-10-20.log`。
- 日志中 `host-5`、`host-6` 的 `session.state` 没有响应。
- 同一日志记录 `session.stream.update` 长时间无响应。

另一条触发路径：

1. Host 重启后，页面恢复了 9 个标签。
2. 选择 D2 会话 `s-1792482626506121700`。
3. `session.state` 再次超过 60 秒不返回。
4. `session.workspace.list` 超时。
5. 重启 Host 后恢复。

因此问题不只在刷新风暴。
大 State 叠加多个 pane 也能触发。

初步分析：

- 每个 `stream.update` 都同步触发一次 `host.ui.command`。
- 调用点在 `plugins/session/ui_state.go:362`。
- UI 发布接口在 `pluginsdk/webui.go:54`。
- Host 每连接队列为 64 帧，满了就丢帧。
- 代码在 `internal/webui/publisher.go:40` 与 `internal/webui/publisher.go:110`。
- 当前没有流式帧合并，也没有发布背压。
- session 插件是单进程，阻塞后所有会话方法一起停住。

预期行为：

- 刷新只影响浏览器连接。
- 后端 Turn 继续执行。
- session 能力保持可响应。
- 页面重连后能恢复 State。

实测行为：

- 后端 Turn 没有继续落 Fact。
- session 能力整体不可用。
- 页面无法恢复。
- 需要停止并重启 session 插件。

### I-02 失败 Turn 刷新后不可见

级别：P1。

复现：

1. 停止 `llm-openai` 插件。
2. 在会话 `s-1792482331250381200` 发送消息。
3. 页面底部出现 `pipeline loop for llm.complete: pipeline_loop`。
4. 刷新页面。
5. 聊天区只剩用户消息。
6. 没有助手消息，也没有失败提示。

JSONL 记录：

- `seq=15` 为 `turn_end`。
- `meta.reason="error"`。
- `meta.error="pipeline loop for llm.complete: pipeline_loop"`。

影响：

- 用户无法在刷新后看到失败原因。
- 错误文案是内部术语。
- U4 的“失败 Turn 可见且可理解”不满足。

根因：

- 聊天区只在过程消息前插入折叠头。
- 过程消息只包括 assistant 与 tool。
- 失败轮只有用户消息，没有过程消息。
- 因此没有折叠头，也没有状态行。
- 相关代码在 `plugins/session/ui/main.js:2224`。
- 修复研究见 [修复方案](2026-10-20-full-test-fix-plan.md)。

补充：

- 失败后输入框内容保留。
- 重启 `llm-openai` 后重试可以恢复。
- 重试没有丢失草稿。

### I-03 设置页修改默认模型后，新会话仍用旧模型

级别：P1。

复现：

1. 打开插件中心 → 设置 → `llm-openai`。
2. 添加模型 `mimo-v2.6-flash-alt`。
3. 将默认模型改为该模型并保存。
4. 设置页显示“已保存”。
5. 左侧“新会话模型”下拉出现新选项。
6. 下拉当前值仍是 `mimo-v2.6-flash`。
7. 新建会话仍使用旧模型。

对照：

- `llm.settings.get` 返回的默认模型已是新值。
- 已有会话继续使用原模型，符合预期。
- 手动在下拉中选择新模型后，新会话会使用新模型。

相关代码：

- `plugins/session/ui/main.js:1245`。
- `applyCatalog` 在默认模型变化时，不会覆盖已有 `selectedModel`。
- 只有当前选中项不在目录中时，才会改用默认模型。

影响：

- 用户修改默认模型后，新建会话不会生效。
- U7 的“新会话使用新模型”不满足。

### I-04 切换工作区后仍选中旧工作区会话

级别：P1。

复现：

1. 在 `my-go-lite-agent` 工作区选中会话。
2. 将“当前工作区”切换到 `network`。
3. 历史列表只显示 `network` 会话。
4. 中间聊天区仍显示旧工作区会话。
5. 历史列表没有 active 项。
6. 打开的标签同时包含两个工作区会话。

影响：

- 工作区、列表、聊天上下文不一致。
- 用户难以判断当前消息会写入哪个工作区。
- U8 的“无跨工作区会话”不满足。

备注：

- 代码注释表明跨区保留是刻意设计。
- 注释位置在 `plugins/session/ui_state.go:569`。
- 另一处说明在 `plugins/session/ui_state.go:994`。
- 当前行为与 PRESSURE U8 判据冲突。
- 修复前需要先确定产品语义。
- 方案比较见 [修复方案](2026-10-20-full-test-fix-plan.md)。

### I-05 插件重启后页面仍显示“生成中”

级别：P1。

复现：

1. W1 Turn 运行中停止 `session` 插件。
2. 插件重启后，JSONL 已有 `turn_end(error)`。
3. 页面仍显示“生成中…”。
4. 停止按钮仍可用。
5. 手动刷新后，Turn 才显示“出错”。

补充：

- session 插件启动时会补齐未闭合 Turn。
- 相关代码在 `plugins/session/store.go:233`。
- 补齐结果是 `turn_end(error)`。
- 文案为 `agent exited before turn_end`。
- 但页面没有自动重新读取状态。
- 插件重启不会触发 UI 的 hello/resync。

影响：

- S8 的“resync 一致”不满足。
- 用户必须手动刷新才能看到终态。

### I-06 取消 Turn 无标记，旧请求进入下一轮上下文

级别：P2。

现象：

1. 发送约 1500 字长文任务。
2. 在流式阶段点击停止。
3. JSONL 记录 `turn_end(reason=cancelled)`。
4. 聊天区没有“已取消”标记。
5. 已流出的部分助手内容被保留。
6. 刷新后内容仍存在。

后续影响：

1. 恢复 `llm-openai` 后发送“请回答 2+2”。
2. 模型先继续写上一轮被取消的长文。
3. 模型推理中引用了旧的取消请求。
4. 新请求被当作附加任务。

影响：

- 用户无法区分“完成”和“被取消”。
- 被取消的请求仍会影响下一轮。
- 客户端/服务端都没有取消标记或提示。

### I-07 慢连接丢帧后可能长期停在旧状态

级别：P2。

复现：

- 打开两个 `bridge/ui` 连接。
- 其中一个暂停读取约 3 秒。
- 期间追加 57 条消息。
- 恢复读取后，慢连接只收到 52 个 chat append 帧。
- 服务端记录 `ui: queue full id=ui-conn-12`。
- 该连接 `drops=16`。
- 其他连接 `drops=0`。

证据：

- `.tmp-e2e/live/multi-ui.mjs` 输出。
- `.tmp-e2e/live/server-2026-10-20.log`。

影响：

- 慢页面可能停在旧状态。
- 如果没有后续帧，seq 断点不会触发 resync。
- S7 的“慢页 resync”未完全满足。

### I-08 工具错误暴露底层错误文本

级别：P2。

W1 中出现：

```text
tool error: statat test: The system cannot find the file specified.
```

影响：

- 错误文本来自底层文件系统。
- 用户和模型都难以判断下一步。
- 模型随后发送了空 `shell_execute` 调用。

备注：

- 该错误可能来自模型把目录当作文件写入。
- 但工具没有给出可操作的路径提示。

### I-09 W1 模型提前收尾，未完成说明与验证

级别：P2，属于模型行为观察。

结果：

- 生成 `test/snake.py`，大小 12,123 字节。
- `python -m py_compile test/snake.py` 通过。
- `python test/snake.py --selftest` 通过。
- 自检共 19 项，全部通过。
- 只有 `test/` 新增，未改动其他业务文件。
- Turn 结束原因为 `completed`。
- 共 8 个 Step、9 次工具调用、63 条 Fact。
- 用时约 263 秒。

未满足项：

- 没有创建 README。
- 没有在 Turn 内运行基础验证。
- 最终回复先说要补充 README，然后直接结束。
- 没有给出最终文件清单和运行命令。
- 未达到 PRESSURE 期望的 15+ Step、10+ 工具、至少 2 个文件。

结论：

- 产物可运行。
- W1 只算部分通过。
- 提前收尾来自模型决策，不是平台终止。

### I-10 Host 重启后残留 `network error`

级别：P2。

现象：

1. Turn 执行中重启 Host。
2. 重启后页面自动重连。
3. 会话恢复到中断的 Turn。
4. 页面出现 `network error`。
5. Turn 没有折叠行。
6. JSONL 已有补齐的 `turn_end(error)`。

补充：

- 无永久 running。
- 历史会话和选中会话都恢复。
- 自动重连有效。

影响：

- 用户只看到模糊的 `network error`。
- 看不到 Turn 已被标记为错误。

## 4. 通过项与实测

### 4.1 可用性

| ID | 结果 | 实测 |
|---|---|---|
| U1 | 通过 | 发送后 300 ms 内进入“生成中”；Turn 4.8 秒完成；1 条用户 Fact |
| U2 | 通过 | 创建 2 个会话；标签切换正常；搜索过滤正常；刷新后标签和选中恢复 |
| U3 | 通过 | 停止可用；JSONL 记录 `cancelled`；刷新后不回到 running |
| U4 | 失败 | 失败 Turn 刷新后不可见；错误文案不可理解；草稿保留 |
| U5 | 部分通过 | D2 窗口 200 条；加载更早后 351 条；滚动锚点未能验证 |
| U6 | 通过 | 两个真实无头页面同时更新；刷新 A 不影响 B；关闭 A 后 B 继续更新 |
| U7 | 失败 | 默认模型修改不生效；已有会话保持原模型；Key 不回显 |
| U8 | 失败 | 切换工作区后仍选中旧工作区会话 |

U5 说明：

- D2 会话为 `s-1792482626506121700`。
- 初始 `first_seq=53`，`has_more=true`，消息 200 条。
- 点击“加载更早消息”后，消息 351 条。
- `first_seq=2`，`has_more=false`，按钮隐藏。
- Trace 窗口为 200 条 Fact。
- 该阶段浏览器视口为 0×0，无法验证滚动位置保持。

U7 说明：

- 默认模型保存为 `mimo-v2.6-flash-alt`。
- 新会话仍使用 `mimo-v2.6-flash`。
- 设置接口返回的默认模型为新值。
- API Key 输入框为空，占位为“已配置，留空保持不变”。
- `llm.settings.get` 只返回 `configured: true`。

### 4.2 压力与错误

| ID | 结果 | 实测 |
|---|---|---|
| S1 | 失败 | 30 次 200 ms 刷新；最终页面空；session 插件卡死 |
| S2 | 通过 | Turn 中刷新一次；后端继续；无 `turn_end`；刷新后恢复 |
| S3 | 阻塞 | 大 State 与多 pane 触发 P0；完整 State 指标未取得 |
| S4 | 通过 | 96 KiB append 推送 194.3 KiB（2.0×）；1 MiB append 推送 2049.9 KiB；100 帧推送 77.5 KiB |
| S5 | 部分通过 | 1 MiB 工具输出被截断显示；页面保持响应；展开与懒加载未验证 |
| S6 | 通过 | D2 窗口 200 条、约 2 MiB 预算；分页按钮行为正确 |
| S7 | 部分通过 | 广播与关闭隔离通过；慢连接丢 16 帧；UI resync 未验证 |
| S8 | 部分通过 | 插件/ Host 重启会补齐 `turn_end(error)`；页面需手动刷新才更新 |

S4 细节：

- 20 次 append，每次新增 96 KiB。
- 每次推送 194.3 KiB，包含 chat、trace、history。
- 推送倍率为 2.0×，低于 2.2× 目标。
- 1 MiB append 推送 2049.9 KiB。
- 100 个流式帧推送 77.5 KiB。
- 快速客户端游标连续。
- 慢客户端出现队列丢帧。

S7 细节：

- 两个客户端都收到 hello 与广播。
- 关闭 A 后，B 继续收到更新。
- 重连 A 后，首帧为 hello，无历史重放。
- 慢客户端收到 52 个 chat append 帧，期望 57 个。
- 服务端记录 `drops=16`。

S8 细节：

- 停止 session 插件时，agent 的 `turn_end(error)` append 返回 `plugin_not_ready`。
- 重启 session 插件后，启动对账写入 `turn_end(error)`。
- 错误为 `agent exited before turn_end`。
- 插件重启后页面仍显示“生成中”。
- 手动刷新后显示“出错”。
- Host 重启后自动重连，历史与选中会话恢复。
- 中断 Turn 在 Host 重启后补齐 `turn_end(error)`。

## 5. 证据索引

| 路径 | 内容 |
|---|---|
| `.tmp-e2e/live/server-2026-10-20.log` | 主测试日志、连接与丢帧记录 |
| `.tmp-e2e/live/server-restart.log` | 插件卡死后重启日志 |
| `.tmp-e2e/live/server-clean.log` | 干净重启与性能负载日志 |
| `.tmp-e2e/live/server-final.log` | 最后重启日志 |
| `.tmp-e2e/live/after-refresh.png` | 30 次快速刷新后的空页面 |
| `.tmp-e2e/live/cdp-refresh.mjs` | 30 次 200 ms 刷新脚本 |
| `.tmp-e2e/live/cdp-two-pages.mjs` | 双页广播测试 |
| `.tmp-e2e/live/multi-ui.mjs` | 双 UI 连接与慢连接测试 |
| `.tmp-e2e/live/measure-2333.mjs` | Append 与流式推送字节测试 |
| `dist/plugins/session/sessions/s-1792483037139265900.jsonl` | 刷新风暴中的 W1 |
| `dist/plugins/session/sessions/s-1792483647755490000.jsonl` | 完成的 W1 |
| `dist/plugins/session/sessions/s-1792484370081969700.jsonl` | Host 重启中的活动 Turn |
| `test/snake.py` | W1 产物 |
| `docs/design/webui-long-session-performance.md` | 既有性能设计与预算 |

证据目录在 `.gitignore` 中，不进入版本库。

## 6. 未执行与阻塞

| 条目 | 状态 | 原因 |
|---|---|---|
| S9 | 未执行 | 需要 50 个会话、10 个工作区、2 小时；当前测试窗口不足 |
| S10 | 阻塞 | 缺少队列指标；只能看到 `queue full` 日志 |
| S3 | 阻塞 | D2 大 State 与多 pane 会触发 P0；完整 State 指标未取得 |
| U5 滚动锚点 | 未验证 | 当时浏览器视口为 0×0 |
| S5 展开与懒加载 | 未验证 | 只验证了截断显示和页面响应 |
| 真实双标签页 | 部分替代 | 内置浏览器不允许 iframe；弹出页被拦截；改用两个无头页面 |

双页测试使用两个真实无头 Chrome 页面。
两个页面都运行完整 WebUI 与应用代码。
不是两个内置浏览器标签。

## 7. 环境恢复

- `llm-openai` 配置已恢复为原模型。
- API Key 长度未变，值未改变。
- `test/` 按 PRESSURE 要求保留。
- 新增测试会话保留在 `dist/plugins/session/sessions/`。
- 测试用 Host 已在测试结束后停止。
- 无头 Chrome 测试进程已停止。
- 未修改业务源码。

## 8. 复现命令

静态检查：

```bash
go vet ./...
go test ./... -count=1
node --test internal/webui/*.test.mjs
powershell -File scripts/build.ps1 -Verify
```

启动 Host：

```bash
cd dist
./liteagent-server.exe
```

快速刷新：

```bash
node .tmp-e2e/live/cdp-refresh.mjs 30 200
```

双页广播：

```bash
node .tmp-e2e/live/cdp-two-pages.mjs
```

双连接与慢连接：

```bash
node .tmp-e2e/live/multi-ui.mjs
```

Append 与流式字节：

```bash
node .tmp-e2e/live/measure-2333.mjs
```

W1 产物验证：

```bash
python -m py_compile test/snake.py
python test/snake.py --selftest
```
