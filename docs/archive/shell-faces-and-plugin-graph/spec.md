# Shell faces、Plugin Graph 与插件文档

## Intent

四条并行需求，落点都在「Host 只留几何、内容归插件」这条主线上：

1. 插件文档补齐 + 无用插件标记 + 构建脚本与源码树对齐
2. Web 前端交互：删除独立 trace 页；初始/新建会话面不加载会话（工作区选取 + agent 模式 + 聊天框），会话按工作区路径分组
3. Plugin 依赖图启动即完整渲染（不再等第一轮对话的 ensurePlugins）
4. Host 底栏信息区，供插件展示信息（工作区、token、模型名、模型总时长）

## Decisions

见 ADR-0024（statusbar 槽位 + New-session Face + 删 trace 页）与 ADR-0025（Plugin Graph 取 Discovery 全量目录 + state）；术语进 CONTEXT.md。

| 项 | 结论 |
|----|------|
| 插件 README | 21 个插件目录全部有 README；新增 `plugins/README.md` 汇总并标状态 |
| 无用插件 | 测试夹具 8 个不发布；`echo` 判定为冗余样例；`echotool`/`uidemo` 为示例 |
| 构建脚本 | 插件目录即真源：只编译+拷贝，不再生成 manifest；build.ps1 与 build.sh 对齐 |
| trace 页 | 删除 `/trace` 页、embed、路由与入口模块；保留中心栏 Session Trace |
| 初始态 | Session View 停在新会话面；Current Session 不再隐式进入（Shell 静默记录） |
| 工作区选取 | `showDirectoryPicker` + `/api/workspace/resolve`（目录名→绝对路径） |
| 分组 | 会话栏只按 Workspace 路径分组，不做 Subagent 树嵌套 |
| 依赖图 | 节点集 = Discovery 目录；state = mounted / available / degraded / missing；新增 depends-on 与 scheme 边 |
| 底栏 | Layout `statusbar` 槽位；session / context-manager / llm-openai / agent 各挂一个 status 组件 |

## Acceptance

- 冷启动打开页面：`/api/plugins` 即含全部 13 个已发现插件（4 mounted + 9 available），图不出空白
- 首轮对话后：scheme 的 `dependsPlugins` 被 ensure，图刷新为 10 mounted（`echo` / `echotool` / `uidemo` 仍 available）
- 新建会话：选工作区→发送首条消息→`session.create` 带工作区、视图转聊天面；会话栏按路径分组
- 底栏四类信息可见并随 Turn 更新（含模型总时长）
- `go build ./...` 与 `go test ./...` 全绿

## Out of scope

- 底栏由 Host 聚合数据（否决，见 ADR-0024 Considered Options）
- 懒 spawn 策略乙、插件卸载（沿用 ADR-0023 的 v1 边界）
- `.scratch` 之外的 issue tracker 集成
