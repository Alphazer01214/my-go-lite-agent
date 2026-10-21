# 2026-10-21 压力测试执行记录

> 状态：仅参考
> 说明：U9、U10、S11、S12 的执行证据。测试规程与条目清单以根目录 `PRESSURE.md` 为准。

本文件是过程记录，不是规范。条目清单、数据档位、监控要求都写在根目录 `PRESSURE.md`，改测试内容改那里。

## 1. 当时的脚本规划

本轮之前规划过一套辅助脚本，放在 `scripts/pressure/`，用于造数据、构造场景和采集证据。该目录最终没有落地，仓库里也没有对应文件，所以下面的命令都无法直接执行：

| 规划脚本 | 用途 |
|---|---|
| `server.mjs` / `chrome.mjs` | 启停被测服务器和 CDP 浏览器 |
| `diagnostics.mjs` / `cdp-eval.mjs` | 读 bootstrap 诊断列表和页面诊断面板 |
| `probe-plugin.mjs` | 建删探针插件 |
| `plugin.mjs` | 插件启停与重新扫描 |
| `turn.mjs` | 发起与取消 `loop.turn` |
| `session-fixture.mjs` | 造 D0 到 D4 数据档位 |
| `cdp-refresh.mjs` / `cdp-tabs.mjs` | 刷新风暴和多标签一致性 |
| `measure-stream.mjs` | 帧数、字节、State 大小与延迟 |

计划约定：直接跑 `dist` 产物，端口 `127.0.0.1:2333`，真实 Chrome，CDP 端口 `9223`；日志、快照和 PID 落在 `.tmp-e2e/pressure/`。

## 2. U9 运行错误进入诊断面板

版本: `93f0e45-dirty`；`bash scripts/build.sh` 重建，`session` 重编译，其余按印记跳过。
环境: Windows 10.0.26200 x64；`dist/liteagent-server.exe`；真实模型 `mimo-v2.6-flash`；基线诊断 1 条（`shell-tools` 的 `manifest_missing`），mounts 4。
数据: D1，已有会话 `slow-close`，102 Facts。

步骤: 备份 `dist/plugins/llm-openai/config.json` → 设置页把 `base_url` 改为 `http://127.0.0.1:9/v1` 并保存 → 发送一条消息 → 观察面板、bootstrap、stderr、聊天气泡 → 逐字节还原配置。

结果: 失败

现象:

- LLM 失败确实发生并被记录。stderr 出现 `err=llm_upstream`，`turn_end` fact 写入，聊天区出现红色 `turn-status-row error` 和 `.error` 行，文本为 `dial tcp 127.0.0.1:9: connectex: ...`。
- 诊断面板没有任何新增。计数保持 1，`GET /bridge/bootstrap` 的 `diagnostics` 保持 1 条，仍是基线那条 `manifest_missing`。
- `/bridge/ui` 上 `type:"diagnostic"` 帧数量为 0。
- 主机状态徽标保持「已连接」，`llm-openai` 保持 active。

指标: 面板 1 → 1；bootstrap `diagnostics` 1 → 1；diagnostic 推送帧 0；stderr 相关 7 行。

原因: 运行期错误没有任何生产者接入诊断。`Server.PushDiagnostic` 只有 3 个调用点，全在 `*_test.go`；`internal/host/ui.go:7-10` 的 `UIPublisher` 只暴露 `PublishState` 和 `PublishCommand`，没有 diagnostic 接口。`internal/host/pipeline.go:287-289` 对插件错误只做透传，`internal/webui/bridge.go:100-103` 只把错误作为 res 帧返回浏览器，两处都不写诊断。稳定错误码 `llm_upstream` 也没有进 `turn_end` fact 的 `meta`，fact 只带 `retErr.Error()` 字符串。

证据: server.log、bootstrap JSON、浏览器 DOM 断言。

备注: 本轮只如实记录现象，未改代码。

## 3. U10 无效挂载不阻塞其他插件

前置同上。探针 `dist/plugins/diag-probe/`，`ui.entry` 指向不存在的 `ui/missing.js`。

结果: 部分通过

通过项:

- 产生 `ui_entry_missing`，`path` 为 `...\dist\plugins\diag-probe\plugin.json#ui.entry`，`message` 是真实 `os.Stat` 错误。
- 探针未贡献任何 mount，其余 6 个插件全部仍挂载，`session` 的 4 个 mount 不受影响。
- 移除探针并重新扫描后，服务端诊断回到 1 条，`session-dock` 回到 `bottom`。

失败项 A（面板陈旧）:

- 放入探针并重新扫描：服务端 `diagnostics` 为 3 条，面板只显示 2 条，缺 `ui_entry_missing` 和 `ui_slot_duplicate`。
- 移除探针并重新扫描：服务端为 1 条，面板仍显示 4 条，含已删除探针的两条。
- 只有整页刷新才同步。根因是 `rescanPlugins` 调用 `applyBootstrap` 但不回填 `serverDiagnostics`，只有 `fetchBootstrap` 会写该数组。

失败项 B（stderr 缺失）: `scan:` 行数量在两次重新扫描后仍为 1，即启动时那一条。扫描期新增诊断不打印命令行，与 `docs/platform/webui.md:397`「命令行窗口始终显示完整诊断」不符。

观测项 C（归属方向）: 探针声明了已被 `session` 占用的 `bottom` slot，`ui_slot_duplicate` 记在 `session` 的 `plugin.json#ui.components[3]` 上，即被抢占方承担诊断，抢占方无名。此项由探针设计造成，不计为缺陷，但归属可读性值得单独讨论。

指标: 诊断 1 → 3 → 1；面板显示 1 → 2 → 4（陈旧）；mounts 4 → 4。

备注: 探针目录已删除，`dist/plugins/` 无残留。

## 4. S11 前端挂载超时与刷新后计数

前置同上。探针 `dist/plugins/diag-slow/`，`ui/main.js` 可 import 且执行，但从不 `customElements.define`。

结果: 通过

- 约 10 秒后出现 `ui_mount_failed`，`message` 为 `custom element diag-slow-box definition timed out`，无需 devtools 即可读懂。
- 模块确实执行（`window.__diagSlowLoaded` 为 1），元素未定义，未挂载。
- `session-history`、`session-chat`、`session-trace` 正常，不受影响。
- 整页刷新后同一诊断仍只有一条，合并计数为 1，未出现 `(2)`。

指标: 刷新后面板 4 条（3 条服务端快照加 1 条本地超时），无重复计数。

备注: 本地残留与快照重填导致 `(2)` 的风险在当前代码下不可达。`ui_mount_failed` 只进 `localDiagnostics`，Host 不知道它，快照里不存在同一 `plugin + path + code`，因此没有重合对象。该风险要等诊断接入运行期错误后才会成为可测项。

## 5. 视觉项阻塞

内嵌 Browser 视口为 `0x0`，`document.visibilityState` 为 `hidden`，截图不可用。面板 CSS 为 `position: fixed; top: 60px; right: 12px; width: min(420px, calc(100vw - 24px))`，在 0 宽视口下 `100vw - 24px` 为负，`getBoundingClientRect()` 得到 2×2 和 `x: -14`。这是量测环境造成，不是布局缺陷。

因此 U9 第 4 步「面板视觉呈现」、非模态、无遮罩、Esc 与关闭按钮的可见行为，本轮只以 DOM 断言完成，未做像素级确认，标记为阻塞。面板打开后 `document.activeElement` 为 `#diagnostics`（元素带 `tabindex="-1"`），与 `docs/platform/webui.md:390`「不抢占键盘焦点」的判定需要可见视口再评。

## 6. 修复后复测：运行期错误接入诊断

改动:

- `internal/webui/bridge.go`：新增 `callDiagnostic`，在浏览器调用失败的两条出口上报诊断——Go 层 `callErr` 和插件返回的错误帧 `Frame.ErrorCode`。前者覆盖路由、插件不可用、Host 关闭等，后者覆盖 `llm_upstream` 这类插件域错误。
- `internal/webui/server.go`：把 `Server.PushDiagnostic` 接到 `/bridge/call`。
- `internal/webui/bridge_test.go`：覆盖错误帧、Host 层错误、成功响应、取消四条路径。
- 未改 `protocol`、SDK 和任何插件。

U9 复测: 通过

- 同一构造（`base_url` 指向 `http://127.0.0.1:9/v1`）发一轮 Turn。
- `GET /bridge/bootstrap` 从 1 条变为 2 条，新增 `plugin=loop path=loop.turn code=llm_upstream`，`message` 为 `Post "http://127.0.0.1:9/v1/chat/completions": dial tcp ...`。
- 面板在不刷新的情况下从 1 变为 2，说明 NDJSON 推送已到达。
- 聊天区红色错误行仍然保留，两个界面同时可见。
- 刷新后诊断仍在快照里。

取消路径: 通过，但触发方式与预期不同

- Turn 运行中点停止后，`loop.turn` 返回的是成功帧（`err=` 为空，payload 含 `cancelled:true`），不是 `call_cancelled` 错误帧。因此端到端没有经过过滤分支。
- 诊断计数保持不变，没有新增条目。
- `call_cancelled` 的过滤由单元测试覆盖；它在 Host 层取消（关闭 Host、路由超时等）时才会走到。

指标: bootstrap `diagnostics` 1 → 2；面板 1 → 2（实时）；取消后保持 2。

备注:

- 构建脚本在 Windows 上存在印记缺陷。`local_dep_dirs` 用 `$ROOT/` 前缀过滤 `go list` 输出，而 `go list` 返回 `E:\...` 形式，过滤后为空，导致 Go 源码改动不参与哈希，`liteagent-server` 会被误判为 unchanged。本轮通过删除 `dist/.build-cache/liteagent-server.sha256` 强制重建。
- `dist/liteagent-server.exe` 不由当前构建脚本产出，是更早构建留下的陈旧文件，修改时间停在 18:40。直接运行它会得到旧代码。当前脚本产物是无扩展名的 `dist/liteagent-server`。
- U10 的面板陈旧问题未修，仍待处理。

## 7. S12 停止依赖后重启依赖方

改动: `internal/host/lifecycle.go` 的 `StartPlugin` 不再因 `depends_on` 目标未就绪而返回错误，改为写一条 `start warn:` 日志后继续启动。`AutostartClosure` 的开机顺序与闭包保持不变。

前置: `93f0e45-dirty` 重建产物；真实模型；全部插件 autostart 就绪。

步骤:

1. 记录初始状态，6 个插件全部 active。
2. 停止 `session`。
3. 停止 `agent`。
4. 启动 `agent`。
5. 在页面上发送一条消息。
6. 检查诊断面板与聊天区。

结果: 通过

修复前该场景是 `POST /bridge/plugins/agent/start` 返回 HTTP 404，`plugin_action_failed`，消息 `plugin agent dependency session: plugin session not mounted: plugin_not_mounted`，且这次失败在日志里没有任何痕迹，也不进诊断。

修复后实测:

- `start agent` 返回 HTTP 200，`agent` 走到 active，`session` 保持 inactive。
- 日志出现 `start warn: plugin agent dependency session: plugin session not mounted: plugin_not_mounted`。
- 页面发送消息后，面板显示 `session / session.state / method_not_found`，同键重复合并为 `(3)`，另有 `session / session.dock / method_not_found`；聊天区错误行给出同一句 `no owner for session.state: method_not_found`。
- 停止状态不落盘，重启 Host 后 autostart 恢复全部插件。

观测: HTTP 状态码、bootstrap `diagnostics`、`start warn` 日志行、面板合并计数。

证据: verify.log、bootstrap JSON、浏览器 DOM 断言。

备注: 路由失败的码是 `method_not_found` 而不是 `plugin_not_mounted`，因为停止插件会删除路由，`routeOwner` 返回空。前端依赖图此时不会阻断任何操作，只用于提示用户该插件需要哪些同伴。
