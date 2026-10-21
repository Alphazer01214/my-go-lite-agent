# WebUI 刷新与渲染问题排查日志

> 状态：已废止
> 说明：根因分析已被后续修复覆盖，代码引用全部过时。

日期：2026-10-19
范围：Session WebUI 渲染层、Session 插件 UI 推送、工作区切换
状态：根因已定位并复现证实，未修改代码

## 1. 问题简报

现象一：频繁刷新后整页变空。对话区显示“从左侧打开一个会话”，会话列表显示“暂无会话”。

现象二：流式生成期间无法选中文字，也无法展开“思考过程”折叠区。

现象三：操作过程不流畅。

预期行为是刷新后恢复对话内容。流式期间应可选中文字、可展开折叠区。交互应顺滑。

三个现象在同一套推送渲染链路上。现象二和现象三同根因，现象一另有独立根因。

## 2. 复现环境

本机 2333 实例当时未运行。为不干扰真实数据，另建隔离实例复现。

- 服务端：`tmp-repro/liteagent-server.exe`，监听 `127.0.0.1:2399`。
- 插件：只挂载 session 插件，数据目录 `tmp-repro/session-data`。
- 驱动：系统 Chrome 无头模式，经 puppeteer-core 操作真实页面。
- 数据：`session.stream.update` 合成流式帧，`session.append` 落地消息，不接 LLM。
- 脚本：`tmp-repro/repro.js` 至 `repro4.js`，可重复运行。

推送链路为真实链路：合成帧从 Browser Call 进入 Session 插件，经 UI 推送流回到页面，与真实 Turn 完全一致。

## 3. 复现结果

### 3.1 流式期间消息列表全量重建

每个流式帧到达后，页面销毁并重建全部消息节点。实测数据如下。

| 检查项 | 实测结果 |
| --- | --- |
| 节点存活 | 40 个消息节点在 1 帧后存活 0 个 |
| 选中文字 | 选中 76 个字符，1 帧后清空 |
| 展开思考过程 | 折叠区展开后 1 帧被折叠，节点已换新 |
| 渲染次数 | 30 帧触发 30 次全量渲染 |
| 渲染耗时 | 204 条消息时单帧 15.6 ms，p95 为 18 ms |

渲染耗时按 10 帧每秒计算，主线程持续占用约 15%。会话越长越卡。长回答的草稿每帧重新解析全文，成本随长度平方增长。

落地一条消息同样触发全量重渲染。一次 `session.append` 使 Chat、History、Trace 三个面板各重建一次。

### 3.2 工作区错位时刷新即空页

工作区错位指注册表当前工作区与正在查看的会话不属于同一工作区。

复现步骤如下。

1. 查看工作区 A 的会话，对话区有 42 条消息。
2. 在工作区下拉中切到空的工作区 B。
3. 对话区立刻变空，列表显示“暂无会话”。
4. 在此状态按 F5 刷新。
5. 页面加载后仍是空页。

切回工作区 A 后内容恢复，204 条消息完整。错位期间后台向 A 的会话追加消息，空页状态不变。

隐形入口是“添加工作区”。`workspace.create` 会自动选中新工作区，注册表随即与当前会话错位。

### 3.3 其他实测行为

以下行为经实测确认，属于次要发现。

- 正常状态连刷 5 次，内容完整恢复。普通刷新不丢数据。
- 流式帧不改变滚动位置。实测偏移为 0。
- 刷新后推送正常送达，草稿内容继续增长。
- 第二个标签页接管 UI 推送连接后，旧页永久冻结，残留“生成中”草稿。恢复只靠手动刷新。
- 生成期间输入框禁用。边生成边打字做不到。

## 4. 根因

### 4.1 流式帧触发消息列表全量重建

Agent 侧对推送频率做了节流。`plugins/agent/stream.go` 第 31～58 行限为 100 ms 或 24 个分片一次。每次推送走 `stream` 命令到前端。

前端 `plugins/session/ui/main.js` 第 956～963 行的 `runCommand("stream")` 每帧调用 `renderMessages()`。该函数第 1119～1143 行开头执行 `replaceChildren()`，随后按 `messageNode()`（第 1064～1097 行）重建每条消息。

三个症状由此产生。

- 节点全部换新，浏览器的选区随之销毁，故无法选中文字。
- `<details>` 折叠区换新后回到收起态，故无法展开思考过程。
- 每帧重建整个列表并重新解析 Markdown，故操作不流畅。

草稿节点有增量更新逻辑（`renderDraft`，第 1145～1180 行），但 `renderMessages` 在开头把草稿引用清空，增量逻辑每帧都被绕过。

对照组是 Trace 面板。它按 `seq` 复用节点（第 1343～1358 行），展开状态能跨推送存活。消息列表缺同款处理。

### 4.2 推送的 selected_id 与 panes 不自洽

`plugins/session/ui_state.go` 第 550～562 行的 `publishPanes` 按注册表当前工作区过滤推送的 panes，`selected_id` 却取全局会话选中。两者错位时，`selected_id` 不在 panes 列表内。

前端 `plugins/session/ui/main.js` 第 927～929 行发现 `selected_id` 缺席后回退为空。第 1129～1133 行据此显示“从左侧打开一个会话”。History 面板同理显示“暂无会话”。

刷新是触发点而非根因。页面加载时三个挂载各调一次 `session.state`。`plugins/session/ui_state.go` 第 403～442 行的 `state()` 是读接口，却带副作用。错位时 `effectiveID` 找不到匹配会话，返回空状态，还会把服务端 `p.selected` 清成空串。

另有两个放大因素。

- `workspace.select` 只改注册表工作区，不改会话选中（`plugins/session/workspace_binding.go` 第 77～83 行）。
- `workspace.create` 自动选中新工作区（`plugins/session/main.go` 第 150 行）。

## 5. 已证伪的假设

排查中证伪了三条假设，记录如下。

- 滚动位置跳动：实测偏移为 0，浏览器在同一任务内保住滚动位置。
- 普通刷新丢内容：正常状态五连刷均完整恢复。
- 刷新后推送失效：刷新后流式帧正常送达。

## 6. 修复建议

按用户要求，本轮只排查不改代码。建议按以下顺序实施。

1. 渲染增量更新。消息节点按 `seq` 复用，参照 Trace 面板做法。流式帧只更新草稿文本节点，`runCommand` 不再触发全量 `renderMessages`。预期同时解决现象二和现象三。
2. 清空防护。`publishPanes` 的 `selected_id` 与 panes 集合保持自洽。前端不被推送顶掉当前查看的会话。`state()` 去掉副作用。预期解决现象一。
3. 交互项。生成期间放开输入框，只禁用发送按钮。工作区下拉改为按项更新。

回归验证用本文附录脚本。修复前脚本变红，修复后同脚本变绿。

## 7. 附录

复现脚本曾放在 `tmp-repro/`，用法如下。该目录是临时环境，已在修复完成后整体删除。复现与回归过程见《WebUI 刷新与渲染问题修复日志》。

```text
# 启动隔离实例（另开终端保持运行）
pwsh -Command "$env:LITEAGENT_HTTP_ADDR='127.0.0.1:2399'; $env:SESSION_DATA_DIR='E:\Projects\my-go-lite-agent\tmp-repro\session-data'; E:\Projects\my-go-lite-agent\tmp-repro\liteagent-server.exe"

# 运行复现脚本
cd E:\Projects\my-go-lite-agent\tmp-repro
node repro.js
node repro2.js
node repro3.js
node repro4.js
```

关键数据出处如下。

- 节点存活、选中丢失、折叠重置：`repro.js` 的 T1a、T1b、T1c。
- 渲染次数与耗时：`repro.js` 的 T2，`repro2.js` 的 T9R。
- 工作区错位清空：`repro2.js` 的 T4R，`repro3.js` 的 T4Rb、T4Rc。
- 证伪数据：`repro4.js` 的 S1、S3。

脚本依赖 puppeteer-core 与系统 Chrome。依赖与脚本随临时环境一并删除。
