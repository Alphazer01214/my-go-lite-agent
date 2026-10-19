# WebUI 刷新与渲染问题修复日志

日期：2026-10-19
范围：Session WebUI 渲染层、Session 插件 UI 推送、工作区切换
状态：修复完成，回归验证通过

## 1. 修复简报

本轮修复三个已定位问题。定位过程见《WebUI 刷新与渲染问题排查日志》。

- 现象一：频繁刷新后整页变空。
- 现象二：流式生成期间无法选中文字，无法展开思考过程。
- 现象三：操作过程不流畅。

根因有两个。流式帧触发消息列表全量重建，导致现象二和现象三。推送的 `selected_id` 与 panes 集合不自洽，导致现象一。修复分别落在前端渲染层和 Session 插件推送层。

## 2. 修复内容

### 2.1 流式帧只更新草稿

`runCommand("stream")` 不再调用全量的 `renderMessages()`，只调用增量的 `renderDraft()`。流式帧到达时只替换草稿文本节点，已落地消息的 DOM 完全不动。

附带修复一处草稿串块。同一会话里换 Turn 或 Step 时，旧草稿块整体换新，上一步的思考内容不会残留。

### 2.2 消息节点按 seq 复用

`renderMessages()` 增加 `syncMessageNodes()`，按 `seq` 缓存消息节点，做法与 Trace 面板一致。三个要点如下。

- 内容未变的节点原地保留，不重新挂载。重新挂载会清掉节点内的文字选区。
- 先删过期节点，再补新节点。顺序稳定的追加场景只做插入。
- 只有切换会话时才整体清空容器。

节点复用后，`<details>` 的展开状态、文字选区、复制按钮状态都能跨推送存活。

### 2.3 推送集合与选中自洽

`publishPanes` 和 `state()` 的 panes 集合含义改为两类：当前工作区的打开会话，加上屏幕上正在看的会话。注册表工作区切走后，屏幕会话不再从推送里消失。

`SelectedID` 保证落在 panes 集合内。找不到对应面板时置空，前端只在这种情况下才回退。

前端同步加两道防护。推送仍带着当前会话时不切换视图。`state()` 读操作不再清空服务端选中。

### 2.4 生成期间可输入

`updateControls()` 只在无会话时禁用输入框。生成期间可以继续打字，发送按钮仍按原规则门禁。`send()` 补了 Turn 运行中的校验，堵住回车直发的旁路。

工作区下拉和会话标签页加签名守卫。内容没变就跳过重建，下拉不会被推送打断。

## 3. 回归验证

### 3.1 Go 测试

新增三个回归测试，位于 `plugins/session/ui_state_workspace_test.go`。修复前全部变红，修复后转绿。

| 测试 | 修复前 | 修复后 |
| --- | --- | --- |
| TestPublishPanesKeepsSelectedAcrossWorkspaceSwitch | panes 为空 | 通过 |
| TestStateReadKeepsSelectionWhenWorkspaceMisaligned | chat 为空 | 通过 |
| TestStateReadKeepsScreenSessionOverWorkspaceMemory | 串台到 s2 | 通过 |

全量测试通过。

```text
go test ./... -count=1
```

### 3.2 浏览器复现

用隔离实例加系统 Chrome 无头驱动真实页面，走真实推送链路。同一组脚本在修复前后各跑一遍，对比结果如下。

| 检查项 | 修复前 | 修复后 |
| --- | --- | --- |
| 1 个流式帧后存活的消息节点 | 0 / 40 | 40 / 40 |
| 选中文字跨帧保持 | 丢失 | 保持 76 字符 |
| 草稿思考过程展开跨帧保持 | 被折叠 | 保持展开 |
| 已落地消息展开跨推送保持 | 被折叠 | 保持展开 |
| 30 个流式帧触发全量渲染次数 | 30 | 0 |
| 204 条消息时 50 帧触发全量渲染次数 | 50 | 0 |
| 工作区切换后消息节点 | 0，整页清空 | 42，保持原样 |
| 错位状态下刷新后消息节点 | 0，空页 | 204，完整恢复 |
| 刷新期间与刷新后的流式推送 | 正常 | 正常 |

204 条消息的场景里，事件循环延迟最大 9 ms，无长任务。渲染热点已从流式路径移除。

### 3.3 构建产物同步

用 `scripts/build.ps1` 重建发布布局。会话数据与插件配置保留 3 个会话文件未动。产物更新如下。

- `dist/plugins/session/session.exe`：含推送层修复。
- `dist/plugins/session/ui/main.js`：含渲染层修复，与源码哈希一致。
- `plugins/session/session.exe`：源码树布局同步重建。

## 4. 修复后的预期语义

- 切换工作区后，对话区保留当前会话，直到用户另选。会话列表仍按工作区过滤。
- 刷新页面恢复当前会话与草稿，不因注册表工作区错位而清空。
- 流式生成期间可以选中文字、展开思考过程、继续输入。
- 每个流式帧只重绘草稿。全量重绘只发生在切换会话时。

## 5. 遗留项

以下问题本轮未改，如实记录。

- 双标签页抢占 UI 推送连接后，旧页永久冻结。这是单连接设计（`internal/webui/publisher.go`），改法涉及产品决策，建议单独立项。
- History 与 Trace 面板在消息落地时仍整体重建。频率低，每 Turn 几次，不在流式热路径上。
- 草稿内容变化时仍整段重解析。量级在毫秒级，仅内容变化的帧发生。
- 复现脚本属临时环境，已按要求删除。需要重做回归时，按排查日志附录的方法重建。

## 6. 变更清单

- `plugins/session/ui_state.go`：`effectiveID` 优先屏幕会话；`state()` 不再清空选中；`uiPaneStates` 替代 `chatStates`；`publishPanes` 自洽。
- `plugins/session/ui_state_workspace_test.go`：新增 3 个回归测试。
- `plugins/session/ui/main.js`：节点复用、草稿增量更新、标签页与下拉签名守卫、生成期放开输入。
- `dist/plugins/session/session.exe`、`dist/plugins/session/ui/main.js`、`plugins/session/session.exe`：构建产物同步。
- `docs/progress/2026-10-19-webui-refresh-render-debug-log.md`：附录补脚本去向。

接口与数据模型没有变化。Frame 协议、Session Fact 类型和 UI 组件契约保持不变。
