# 07 — 主会话视图插件化（渲染）

**What to build:** session 插件第三挂载 session-view 组件（主内容槽位）：重放历史事实（capability 调用）+ 实时消费 presentation/stream——markdown_text / message_text / summary_text / stream 的渲染语义与 CLI Medium 一致、不变。Shell 的内建聊天渲染**同票移除**；输入框本期暂留 Shell（08 迁移）。

**Blocked by:** 05

**Status:** resolved

- [x] 对话显示完全由 session-view 渲染，渲染语义与 CLI Medium 一致
- [x] 流式输出、Presentation Card、settle 后全量渲染不回归
- [x] Shell 主内容区不再有内建聊天渲染
- [x] 历史重放走 capability 调用（/api/history 的删除留给 09）

## Answer

session 插件第四挂载 `session-view`（main 页 chat 槽）。chat.js 的全部渲染语义逐行移植进组件 Shadow DOM：dsh 式 disclosure（Thinking/Answer live 草稿、tool 卡、status 行）、markdown_text settle 与流式去重、message_text/summary_text 分类渲染、贴近底部才跟随的滚动。markdown 渲染经平台模块 `import from '/app/md.js'`、reasoning 合并经 `/app/facts.js`——Web Medium 把这两个模块列为平台面（与 SDK 同类），插件不复制实现。

事件通道重构：events.js 缩为「扇出 + composer 状态行 + Send/Stop 态」——presentation/stream/session 主题完全经 LiteAgent 扇出，视图自行订阅并**本地排队**（历史落地前先入队、之后回放）——比旧的「history 先行再开 SSE」全局闸门更严（旧实现 fetch 窗口内的事件会丢）。composer → 视图走私有主题：`__turn-start`（用户回显 + 清理 live 工件）、`__notice`（命令输出/错误）。

退役：shell 内建聊天渲染全链（chat.js 模块、.msg/.disc/.message/.progress CSS、#chat 滚动归属组件、historyReady 闸门）。shell.html 172→约 120 行。历史重放走 `call('session','query')`。输入框仍在 shell（08 迁移）。全量 `-count=1` 绿；全部模块 node --check 过。
