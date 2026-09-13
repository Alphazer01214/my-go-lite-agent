# 08 — 输入框迁移，Shell 归零内容

**What to build:** SDK 增加 send-message 与 run-command 两个通用方法（命令面仍归 Host，任何插件可用）；输入框移入 session-view 组件；layout 不再含任何输入控件与内容组件——「整体 layout + 整体样式表 + 必要全局脚本」达标。默认 assembly 挂载 session Web 面，开箱即用。

**Blocked by:** 07

**Status:** ready-for-agent

- [ ] 输入框属 session-view；layout 零输入控件、零内容组件
- [ ] 消息发送与 / 命令经新 SDK 方法工作；命令冲突与保留名行为不变
- [ ] examples 默认 assembly 开箱即用（聊天、命令、切会话、trace 全通）
- [ ] Shell 仅剩 layout/槽位、Design Token 样式表、SDK 与装载器
