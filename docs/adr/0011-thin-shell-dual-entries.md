# Shell 缩薄：layout 之外皆插件，Web Medium 内建于 server 入口

程序收敛为双入口：liteagent-cli.exe（内核 + CLI Medium）与 liteagent-server.exe（内核 + Web Medium，保留 `-repl` 组合），共享同一内核与全部插件协议，分歧仅在前端 Medium。Web Medium 的 HTTP 静态服务、layout、SSE hub 内建于 server 进程，不做独立插件进程：协议无需为「插件订阅事件流」扩容（protocol 保持 2），代价是改 layout 需重编 server.exe，与 build.ps1 全量构建的日常相符；将来若要「换壳不重编」，再升级为插件进程并补事件订阅设施，升级路径保留。

Shell 缩至最薄：项目至多提供整体 layout（页面与 Panel 槽位）、整体样式表（Design Token）与必要全局脚本（LiteAgent SDK 与通用装载器——读 /api/plugins、动态 import UI Entry、按 page+slot 挂载）。ADR-0009/0010 的「插件不得替换聊天主流程」约束就此废止：聊天主流程移出 Shell，成为插件 Panel Component。官方实现不例外——session 插件（已有 session capability）增加 `ui` 块，声明主会话视图（主内容槽位）、会话栏（sidebar）与 trace 视图（trace 页）三个挂载。不设「chat 插件」：聊天面在协议里从无对应 capability，/api/history 与 /api/trace 经查证同为 `QuerySessionFacts` 的投影，聊天面就是 Session Log 的主视图，「聊天」降为口语别名。可替换性由槽位 + Assembly 保证：第三方视图声明 mount 到同一槽位即可竞争。

数据通道随之归位：Session View 经 SDK `call('session','query',…)` 自取事实，/api/history、/api/trace 两个专用端点退役，Web Medium 对 session 的依赖只剩通用协议；投影逻辑（事实→消息流）归组件。「当前会话」是媒介级状态，暂留 Web Medium 层（/api/message、/api/session/select），是否下沉为 Capability 另议。SDK 增 send-message / run-command 两个通用方法（命令面仍归 Host），输入框归 Session View 组件，layout 无任何输入控件。

插件资产与声明语义同步扩展：`ui/` 从单 ES Module 扩为多文件资产包（main.js + `<template>` 片段 + CSS，经 Shadow DOM 装载），Manifest 校验扩为「入口与声明文件存在」；layout 带最小页面机制（/ 主页面、/trace 调试页），mounts 增加 page 维度（缺省 main）；UI-only 插件合法化——`entry` 可省，进程为 Frame 而存在，纯视图插件无 Frame 可说。不采纳 manifest 级 `have_web_ui` 布尔：有无 web UI 由 `ui` 块声明（作者事实），是否挂载归 Assembly 配置（用户意图），发现 ≠ 挂载的分工不变。迁移三步：① 拆双入口（机械、行为不变）② Shell 缩薄 + session Web 面（含 UI-only 合法化与专用端点退役）③ 多文件资产契约 + uidemo 迁移。信任模型不变（完全信任、同页执行，ADR-0010）。
