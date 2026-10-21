# 插件中心与 Session 交互改造日志

> 状态：部分有效
> 说明：依赖图的布局与环处理规则仍有效。「启动前校验 `depends_on`」的说法不成立，Host 只告警不拦截，见 `internal/host/lifecycle.go`。

日期：2026-10-16  
范围：插件加载、插件启停、插件中心、依赖关系图、Session WebUI  
状态：实现与浏览器验证已完成

## 1. 背景

本轮要解决三件事。

第一，插件目录需要支持两种加载时机。启动时自动扫描一次。运行期间由用户手动重新扫描。

第二，插件管理入口需要收拢。设置、插件列表和依赖关系应进入同一个菜单。

第三，Session 交互需要更接近常见网页 Chat 和 Desktop Agent。

资源占用和安全加固不在本轮范围。相关问题留到压测和安全专项。

## 2. 改动边界

本轮允许修改：

- Host 插件生命周期；
- WebUI 管理接口；
- Host Shell 插件中心；
- Session Component；
- 对应测试和文档。

本轮不修改 Frame 线协议。插件管理继续走独立 HTTP 管理面，不进入 Browser Call 的 `host.*`。

本轮不增加 ZIP 插件包、文件监听、自动重启和启停持久化。

## 3. 插件加载与启停

### 3.1 加载时机

启动流程保持不变：

```text
ScanRoot
→ AutostartClosure
→ 启动 autostart 闭包
```

运行期间新增手动扫描：

```text
POST /bridge/plugins/rescan
→ Host.RescanPlugins
→ plugin.ScanRoot
→ 更新 Discoveries
→ 更新 WebUI Registry
→ 返回 Bootstrap
```

手动扫描只更新发现结果。它不会自动启动新插件。

新插件先进入 `available`。用户随后在插件中心手动启动。

### 3.2 生命周期 seam

Host 新增窄接口：

```go
type PluginLifecycle interface {
    PluginSnapshot() []PluginSnapshot
    RescanPlugins() (plugin.Discoveries, error)
    StartPlugin(name string) error
    StopPlugin(name string) error
}
```

WebUI 只依赖该接口。它不读取进程、stdin、命令对象和 generation。

启动前检查 `depends_on`。依赖未挂载时返回错误，不启动无效插件。

停止过程先闭环 pending，再关闭 stdin。进程超过宽限期仍未退出时强制结束。

停止不级联影响依赖方。依赖方由状态快照派生为 `degraded`。

### 3.3 状态语义

| 状态 | 含义 |
|---|---|
| `mounted` | 进程已挂载且健康 |
| `available` | 可启动，或已被手动停止 |
| `degraded` | 进程不健康，或计划启动但启动失败 |
| `disabled` | 持久禁用状态，当前实现未写入 |
| `missing` | 被依赖插件未发现 |

手动停止只保存在当前 Host 内存。Host 重启后重新按 `autostart` 计算。

## 4. 统一插件中心

顶栏不再分别显示 Settings 和 Plugins。现在只保留一个 `插件中心` 入口。

插件中心包含三个页签：

| 页签 | 内容 |
|---|---|
| 插件 | 扫描、启停、状态、Capability、依赖和 Mount |
| 设置 | Manifest Settings Schema |
| 依赖关系 | SVG 依赖图和文本边列表 |

插件页新增“重新扫描”按钮。每行根据状态显示“启动”或“停止”。

管理请求期间禁用当前按钮。状态提示使用 `aria-live`。失败后保留原按钮，允许重试。

管理接口统一返回 Bootstrap。前端据此更新插件快照和新增 Mount。

## 5. 依赖关系图

依赖图只读取 Bootstrap 中的 `depends_on`。箭头由依赖方指向被依赖插件。

本轮示例中：

```text
agent → session
agent → llm-openai
```

布局按依赖深度分层。无依赖插件位于左侧。依赖方位于右侧。

缺失依赖也会生成节点。节点状态显示 `missing`。

SVG 由 DOM API 构造。代码没有拼接 `innerHTML`。图下保留文本边列表，便于阅读和测试。

循环依赖不会造成递归失控。深度计算遇到 DFS 栈内节点时回退为零。

## 6. Session 交互

### 6.1 布局

消息区位于中间。Composer 固定在消息区底部。

输入框支持自动增高。Enter 发送。Shift+Enter 换行。

生成中显示状态。按钮切换为“停止”。取消请求期间继续显示进行中状态。

### 6.2 会话历史

历史列表新增搜索框。搜索范围包括标题、Session ID 和 Workspace。

搜索只过滤显示结果。它不改变 Runtime 的当前选中会话。

切换 Session 时按 Session ID 保留未发送输入。刷新页面后草稿仍会丢失。最终正文仍由 Fact 恢复。

### 6.3 消息展示

用户消息和助手消息增加视觉区分。消息头显示角色和时间。

消息正文支持复制。代码块提供独立复制按钮。

assistant reasoning 使用折叠区。工具调用 JSON 也使用折叠区。

正文继续使用 DOM 节点构造。插件 HTML 不会被执行。

### 6.4 滚动与状态

用户离开底部后，新推送不强制拉回底部。页面显示“回到最新”。用户点击后才滚动到底部。

Session UI State 的 revision 改为单调递增。Component 丢弃同 Session 的旧 revision。

这解决了初始 `session.state` 与 UI Push 竞态时的回退问题。

ChatState 消息新增 `reasoning` 和 `timestamp`。两者都由已有 Fact 投影，不增加持久化格式。

## 7. 文件落点

| 范围 | 文件 |
|---|---|
| 生命周期 | `internal/host/lifecycle.go` |
| 状态快照 | `internal/host/snapshot.go` |
| 管理 HTTP | `internal/webui/management.go` |
| Registry 刷新 | `internal/webui/registry.go` |
| 统一菜单 | `internal/webui/static/index.html` |
| 菜单交互 | `internal/webui/static/app.js` |
| 插件列表与依赖图 | `internal/webui/static/plugins.mjs` |
| Session UI | `plugins/session/ui/main.js` |
| Session State | `plugins/session/ui_state.go` |

## 8. 验证结果

自动测试：

```text
go test ./...
go vet ./...
node --test internal/webui/*.test.mjs
```

结果全部通过。Node 测试共 13 项。

浏览器实测：

- 插件中心三个页签正常切换；
- 设置表单完成加载；
- 依赖图显示两个硬依赖；
- 运时重新扫描返回 3 个插件；
- 停止 `llm-openai` 后状态变为 `available`；
- 再次启动后状态恢复 `mounted`；
- Session Composer 位于底部；
- 会话搜索框正常显示；
- 浏览器控制台无 error 或 warning。

当前扫描仍报告 `context-manager` 和 `filetools` 缺少 `plugin.json`。它们是未形成插件包的目录。诊断行为符合预期。

## 9. 后续工作

本轮暂未处理资源和安全问题。后续压测应覆盖：

- 大量 Browser Call 并发；
- 长流式响应；
- 大 Session JSONL；
- 频繁启停和重新扫描；
- Diagnostic 长时间累积。

安全专项应覆盖：

- 管理接口认证；
- Browser Call 来源校验；
- 插件入口路径约束；
- Session ID 路径约束；
- 非回环监听保护。

插件管理后续还可增加自动重启和启停状态持久化。
