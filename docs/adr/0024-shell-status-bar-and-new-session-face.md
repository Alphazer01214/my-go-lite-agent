# Shell 底栏槽位与 New-session Face

**Status: accepted**

Web Shell 的两处骨架调整，都是「内容归插件、Shell 只留几何」的延续（ADR-0011/0012）：

1. **新增 `statusbar` 槽位**：Layout 的 main 页新增 region=bottom 的 `statusbar` 槽位，Shell 提供整宽底栏几何与分隔样式，内容由插件以 `<plugin>-status` Panel Component 挂载（session 的工作区/会话/事实数、context-manager 的 token 占用、llm-openai 的 model 与 model-time、agent 的 scheme）。信息项是各插件对自己状态的观测投影；Shell 不聚合、不解读，也不新开 Host 横切面。为拿到「模型总时长」，llm-openai 增 `llm.stats`（进程内计数：请求数、总/平均/末次时长、token 数）。

2. **New-session Face 成为 Session View 的初始面**：启动与「＋」都不再加载 Current Session，改为停在「工作区选取（可留空）+ Agent Scheme 选择 + 聊天框」。工作区选取用浏览器文件夹选择 API（`showDirectoryPicker`）；该 API 只给目录名不给路径，故新增 Host 端点 `/api/workspace/resolve` 把目录名映射为绝对路径（cwd 子树 depth≤3，再查祖先的直系子目录；多个同名返回候选，零个则要求手填）。发出第一条消息时才 `session.create`（带 Workspace）并切到会话面。

同时**删除独立 `/trace` 页面**（`web/static/trace.html`、`/app/trace-page.js`、`/trace` 路由与 `web/embed.go` 的 embed）：中心栏的 Session Trace 保留，作为 Session Log 的调试投影唯一入口。独立页面与中心栏同源同粒度，重复且带来第二套 chrome 维护成本。

会话列表（sidebar 的 `session-rail`）的分组依据收敛为 **Workspace 路径**这一条：项目相同即同组，不再按 Subagent 父子做树嵌套（子会话仅保留 `↳` 徽标）。

**Considered Options：** 底栏由 Shell 内建并聚合各插件数据（需要 Host 横切面 + 第二真源，否决）；底栏做成页面级页脚而是另一 page（导航复杂化，否决）；工作区选择仅用文本输入（放弃浏览器 API 的选择体验，否决）；`showDirectoryPicker` 后只存目录名（Workspace 必须是可用路径，否决）。
