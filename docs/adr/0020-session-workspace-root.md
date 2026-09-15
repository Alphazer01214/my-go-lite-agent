# Workspace 作为 Session 元数据

Workspace 是 Session 元数据字段：CLI 进程默认启动 cwd，Web 由会话选择/创建时写入；filetools、shelltools、permission、skill-manager、project-context 一律从当前 Session 读根，不使用进程全局 cwd。

多 Session / Subagent 可指向不同目录；服务端一次进程可同时服务多个项目根。代价是每个工具调用需携带或经上下文解析 sessionId 对应的根——由 agent 在 tools.call payload 注入 `workspace`，工具插件不得自行猜 cwd。
