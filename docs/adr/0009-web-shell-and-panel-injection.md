# Web Shell 与 Panel 同文档注入

> Status: partially superseded by ADR-0010 —— Panel 内容的「同文档 HTML 注入」已由 Panel Component 取代；Shell、默认聊天面与「插件不得替换聊天主流程」约束继续有效。

Web 是第二 Render Medium：Host 内嵌 HTTP（`-serve`），提供 Shell（布局、Panel 槽位、默认聊天面、命令输入、事件桥）。默认聊天面只消费既有 Presentation 信号（markdown_text / message_text / summary_text / stream），插件不得替换聊天主流程。

业务 UI 由插件经 `presentation.panel` 同文档注入（`set|append|clear` + slot + id + html）；完全信任插件作者，无 iframe/CSP。交互经 `cap=ui, method=action` 回传目标插件。前端零 Node：全局 `window.LiteAgent` + 可拷贝 `sdk/lite-agent.js`。SSE 复用 Presentation JSON；多标签 replay 内存 ring buffer。protocol 仍为 2（新 method / 新 cap 向前兼容）。
