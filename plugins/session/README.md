# session

File-backed Session Log 插件：JSONL 追加事实流，提供会话与 Trace 的 Web Panel。

## 提供

- Capability `session`：append / query / derive / info / list / create / current…
- 命令：`/session dump-trace | list | derive | current | info`
- UI：
  - `session-rail` —— 会话列表（sidebar）。**只按工作区路径分组**（ADR-0020）；子会话带 `↳` 徽标，但不做树嵌套
  - `session-view` —— 主会话视图（chat 槽位）。同时承载**新建会话首页**：工作区选取（浏览器文件夹 API + Host 路径解析）+ agent 模式选择 + 聊天框。启动与「＋」都不加载任何会话；只有用户选会话、或发出第一条消息时才 `session.create`（带工作区）并进入会话
  - `session-trace` —— Trace 投影（中心 trace 栏；独立 `/trace` 页面已删除）
  - `session-status` —— Host 底栏信息区的 chip：工作区 / 当前会话 / 事实数

## 配置

无独立 config.json；会话文件在插件目录下 `sessions/`。

## Manifest

- `autostart: true`（核四件之一）
