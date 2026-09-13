# Panel Component 化：插件 UI 从 HTML 注入到原生 Web Components

Web Medium 的 Panel 内容从「同文档 HTML 字符串注入」（ADR-0009）升级为原生 Web Components：Manifest `ui.entry` 指向插件自带的 ES Module（UI Entry），Shell 经 `/plugin-ui/` 动态 import，模块注册 custom elements；静态挂载由 Manifest `ui.mounts`（slot + component + props）声明，运行时变化经 PanelOp `set|clear`（component + props，按 id 重挂载）。样式以 Shadow DOM 隔离，主题经 Shell 的 `--la-*` Design Token 联动；`data-la-*` 全局事件委托退役，组件内部直调 `LiteAgent` SDK。protocol 保持 2。

信任模型不变：完全信任插件作者，无 iframe/CSP——进程插件本已是宿主机上的任意原生代码，为 Web 层单独沙箱没有实际收益。但信任性质从被动 HTML 升级为主动执行插件 JS；不可信第三方插件的 iframe 岛记为未来路径，本期不做。不留 HTML 注入兼容路径：Panel 唯一消费者 uidemo 同期迁移，两种模型长期并存只会让契约含糊。

治理与语义：Host 校验 PanelOp 的 component 名必须以发出插件名 + `-` 为前缀，违规拒绝；模块内 `customElements.define` 为文档化约定，Shell 依赖原生 upgrade 机制容忍「op 先到、模块后加载」。插件变更经广播触发 Shell 整页刷新（状态真源在 Session Log，不做热替换）。聊天主流程仍归 Shell，插件不得替换（ADR-0009 约束继续有效）。
