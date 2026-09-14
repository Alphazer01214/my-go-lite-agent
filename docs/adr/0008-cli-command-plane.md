# CLI Command 平面：Manifest 声明 + Host 路由 + 冲突拒载

主窗口 slash command 分两层：Host 内建原生命令（`/help` `/lp` `/refresh` `/exit`），插件经 Manifest `description` + `commands[]` 声明附加命令。调用形如 `/插件名 子命令 参数…`，Host 路由 `cap=commands, method=call` 到目标插件进程。

Session Log 导出属 session 插件能力（`/session dump-trace`），不是 Host 原生命令——与「一切皆插件」同构。与原生命令同名的插件**整个不予加载**（启动 warn），而不是只禁命令面——保留名冲突属于配置错误，静默砍掉命令面更难排查。Manifest 刷新只更新内存元数据，不做热插拔。
