# 08 — 输入框迁移，Shell 归零内容

**What to build:** SDK 增加 send-message 与 run-command 两个通用方法（命令面仍归 Host，任何插件可用）；输入框移入 session-view 组件；layout 不再含任何输入控件与内容组件——「整体 layout + 整体样式表 + 必要全局脚本」达标。默认 assembly 挂载 session Web 面，开箱即用。

**Blocked by:** 07

**Status:** resolved

- [x] 输入框属 session-view；layout 零输入控件、零内容组件
- [x] 消息发送与 / 命令经新 SDK 方法工作；命令冲突与保留名行为不变
- [x] examples 默认 assembly 开箱即用（聊天、命令、切会话、trace 全通）
- [x] Shell 仅剩 layout/槽位、Design Token 样式表、SDK 与装载器

## Answer

SDK 增 `sendMessage(text, sessionId?)` 与 `runCommand(line)`（`sdk/lite-agent.js` 与 `web/static/sdk.js` 两份拷贝同步，cmp 校验一致）；文档注释同步。composer（textarea + Send/Stop）移入 session-view Shadow DOM：Enter 发送、`_running` 由组件自持（send 置位、status running/idle/error 驱动、切换复位），Cancel 走 `/api/turn/cancel`，命令输出经组件内 appendPre。`__turn-start` 的唯一发射者（shell composer）退役，订阅保留作为外部 composer 通道文档。

Shell 归零：`#composer/#input/#btn-send` DOM 与 CSS 移除；`state.running`/`setRunning`（按钮态）随之消亡——events.js 缩为「扇出 + status 行」，main.js 只剩 boot 编排 + `__session` 标签同步。README Web Medium 章节改为「Shell 只提供 layout/Token/脚本；内容皆插件组件」。

命令冲突与保留名行为不变（Host 命令面未动）。examples 无需改动——session 的 Web 面随挂载自动生效。全量 `-count=1` 绿；全部 JS 模块 node --check 过。
