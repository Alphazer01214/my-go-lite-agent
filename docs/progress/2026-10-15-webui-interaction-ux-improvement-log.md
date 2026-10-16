# WebUI 交互体验改进记录

日期：2026-10-15  
分支：`webui`  
对象：Host WebUI 平台壳 + `session` 插件 WebUI  
状态：两轮改进已实现、已重建、已实测验证。

## 1. 目的与方法

两轮都只处理交互体验，不改插件业务语义。

方法分四步：

1. 用 `dist/minimal` 运行现场启动 Host，访问 `http://127.0.0.1:2333/`；
2. 用浏览器实测完整交互路径，记录问题；
3. 在源码侧实现改进，重建 Host 并同步插件 UI；
4. 复测关键交互，确认改进生效。

问题清单来自实测与源码核对。清单只保留可复现项。

第一轮处理针对可见的交互摩擦。第二轮按「一切皆插件、Host 与插件不耦合」原则复核改动，继续排查状态与推送层面的交互缺陷，见第 7 节。

## 2. 运行环境

```text
运行目录：E:\Projects\my-go-lite-agent\dist\minimal
启动程序：liteagent-server.exe
监听地址：127.0.0.1:2333
插件：agent、llm-openai、session
Mount：session:history -> left，session:chat -> center，session:trace -> right
```

启动后 `GET /bridge/bootstrap` 返回 3 个插件、3 个 Mount、0 条诊断。

## 3. 交互问题清单

| 编号 | 现象 | 位置 | 影响 |
|---|---|---|---|
| P1 | 底部空槽固定 176px，并显示大写占位文字 `BOTTOM` | Host Shell 布局 | 无插件时白占一横条，暴露内部槽位名 |
| P2 | 面板只能靠按钮开合：无 Esc 关闭、无点击外部关闭 | 设置、插件、诊断面板 | 关闭路径单一，误开面板后要先找按钮 |
| P3 | 诊断面板自身没有关闭按钮 | 诊断面板 | 只能回到顶栏再点一次开关 |
| P4 | 诊断按钮计数为 0 时禁用，且无禁用原因说明 | 顶栏诊断按钮 | 用户不知道按钮为何不可点 |
| P5 | 初始化与失败均无反馈，失败只写诊断快照 | 顶栏 | 页面空白时无法判断是加载中还是连不上 |
| P6 | 发送后输入框与发送按钮未禁用，无生成中反馈 | `session-chat` | 可重复提交，状态不可见 |
| P7 | 取消按钮空闲时也可点击 | `session-chat` | 空心按钮误导用户 |
| P8 | 正文按纯文本输出，代码块与标记原样显示 | `session-chat` | 代码和列表不可读 |
| P9 | 消息区固定 `max-height: 48vh` | `session-chat` | 中栏留大片空白 |
| P10 | 会话列表无当前选中高亮，CSS 已定义 `.item.active` 却未使用 | `session-history` | 无法确认当前会话 |
| P11 | 列表项只显示英文 `N facts`，无时间 | `session-history` | 中英混排，缺少排序线索 |
| P12 | Trace 整体用 `JSON.stringify` 输出 | `session-trace` | 长会话不可读，无筛选 |
| P13 | 生成中的 reasoning 通道内容不展示 | `session-chat` | 思考过程被丢弃 |
| P14 | 组件紧贴三栏边缘，无内边距 | 三个 Component | 视觉拥挤 |

## 4. 改进点与改进方式

### 4.1 Host Shell

| 改进点 | 改进方式 | 文件 |
|---|---|---|
| 空槽折叠 | 底部行高度由 `176px` 改为 `auto`，加 `.slot-bottom:empty { display: none }` | `internal/webui/static/shell.css` |
| 去掉英文占位 | 删除 `.slot:empty::before` 规则，槽位名只留在 `aria-label` 并改为中文 | `shell.css`、`index.html` |
| 面板统一关闭语义 | 抽出 `setPanel`、`closePanels`、`togglePanel`；监听 `Escape` 与文档 `click`，命中面板或开关按钮时忽略 | `app.js` |
| 诊断面板可自关闭 | 面板头部加关闭按钮，按钮名与提示文案齐全 | `index.html`、`app.js` |
| 按钮激活态 | 面板打开时给开关按钮加 `is-active`，同步 `aria-expanded` | `app.js`、`shell.css` |
| 焦点管理 | 打开面板时聚焦面板容器，关闭时焦点回落到对应开关按钮 | `index.html`、`app.js` |
| 禁用原因可见 | 计数为 0 时写入 `title="诊断：暂无诊断信息"`，非 0 时写入条数与关闭提示 | `app.js` |
| 空状态文案 | 诊断列表为空时渲染 `暂无诊断信息` | `app.js`、`shell.css` |
| 连接状态反馈 | 顶栏加状态徽标：`连接中…`、`已连接`、`连接失败`，由 bootstrap 结果驱动 | `index.html`、`app.js`、`shell.css` |
| 按钮可发现性 | 三个顶栏按钮补 `title` 说明用途与关闭方式 | `index.html` |

关闭语义的判定顺序很重要。先看点击目标是否落在面板或开关按钮内，是则直接返回，否则关闭全部面板。这样点击开关按钮只走开关逻辑，不会与外部点击逻辑打架。

### 4.2 `session` 插件 WebUI

| 改进点 | 改进方式 | 文件 |
|---|---|---|
| 发送态反馈 | 由 `busy` 与 `turn.status` 共同推导运行态；运行时禁用输入框与发送按钮，显示带圆点的 `生成中…` 提示 | `plugins/session/ui/main.js` |
| 取消按钮语义 | `turn.status` 非 `running` 时禁用；运行中才可点 | `main.js` |
| 输入体验 | 输入控件改多行 `textarea`，高度随内容自适应；Enter 发送，Shift+Enter 换行 | `main.js` |
| 中文输入法安全 | 按键处理跳过 `event.isComposing`，输入法组合期按 Enter 不发送 | `main.js` |
| 失败不丢草稿 | 发送前清空输入框，请求失败时回填原文本 | `main.js` |
| 正文可读性 | 新增安全渲染器：围栏代码块转 `pre.code-block`，行内代码转 `code`，`**粗体**` 转 `strong`，`- ` 转列表，`#` 转为标题 | `main.js` |
| 渲染安全 | 渲染器只构造 DOM 节点，不用 `innerHTML`；用户文本一律走 `createTextNode`，HTML 标签不会变成元素 | `main.js` |
| 布局填充 | 组件 `:host` 改为纵向 flex、`height: 100%`；消息区 `flex: 1` 加自身滚动 | `main.js` |
| 滚动不打断阅读 | 重绘前判断是否贴近底部，只有贴近时才自动滚到底 | `main.js` |
| 会话选中高亮 | 组件记录 `selectedId`，渲染时给对应项加 `active`。第二轮把选中下沉到 Runtime，改为读 HistoryState 的 `selected_id`，见 7.2 | `main.js` |
| 选中保持稳定 | 推送只更新列表，不抢用户选择。第二轮由 Runtime 统一持有，见 7.2 | `main.js` |
| 列表信息 | 列表项改为 `N 条记录 · MM-DD HH:mm`，`last_ts` 缺失时回落到 `created_ts`，完整 id 放入 `title` | `main.js` |
| 会话空态 | 无会话时渲染 `暂无会话` | `main.js` |
| 消息空态 | 无消息且无草稿时渲染 `输入一条消息开始对话` | `main.js` |
| Trace 结构化 | 每条记录渲染为 `details`：`#seq` + 中文类型标签 + 角色 + 内容预览，展开后看格式化 JSON | `main.js` |
| Trace 降噪 | 加类型筛选下拉：`全部`、`仅消息`、`仅过程` | `main.js` |
| Trace 分页 | 首屏渲染 60 条，超出显示 `显示更多（还有 N 条）`，每次再加载 60 条 | `main.js` |
| 思考过程展示 | 流式草稿含 `reasoning` 时，用可折叠的 `思考过程` 区块单独展示 | `main.js` |
| 主题令牌 | 组件配色统一为插件自有的 `--session-*` 变量并带回退值，不引用 Host 令牌 | `main.js` |
| 留白 | `:host` 统一 12px 内边距 | `main.js` |

标记约定保持不变。三个 Component 继续实现 `setData`、`runCommand`、`connectedCallback`、`disconnectedCallback`，标签名与槽位不变。第一轮没有新增 State 或 Command 字段。

Trace 的预览字段要从 `fact.meta` 里取。该字段在浏览器侧是对象而非字符串，因为 Go 侧的 `json.RawMessage` 会按原样序列化。

## 5. 构建与同步

Host 静态资源用 `go:embed`，改 Host 必须重建可执行文件。`session` 插件由文件系统提供，改插件代码要重建插件可执行文件，改插件 UI 只需复制。

```text
go build -o dist\minimal\liteagent-server.exe ./cmd/liteagent-server
go build -o dist\minimal\plugins\session\session.exe ./plugins/session
Copy-Item plugins\session\ui\main.js dist\minimal\plugins\session\ui\main.js -Force
```

重建前要先停掉运行中的 Host，否则 Windows 无法覆盖被占用的可执行文件。

## 6. 第一轮验证

| 验证项 | 结果 | 证据 |
|---|---|---|
| 底部 `BOTTOM` 占位条 | 通过 | `document.body.innerText` 不含 `BOTTOM`；底部行已折叠 |
| 顶栏连接状态徽标 | 通过 | 硬刷新后顶栏显示 `已连接` |
| 聊天输入框提示 | 通过 | 占位文案为 `输入消息，Enter 发送，Shift+Enter 换行` |
| 发送态反馈 | 通过 | 发送中输入框与发送按钮禁用、出现 `生成中…`；结束后取消按钮自动禁用 |
| 面板 Esc 关闭 | 通过 | 打开设置面板后按 Esc，`#settings.hidden` 变为 `true` |
| 点击外部关闭 | 通过 | 打开设置面板后点击左栏空白处，`#settings.hidden` 变为 `true` |
| 开关按钮激活态 | 通过 | 面板打开时按钮 class 含 `is-active` |
| 会话选中高亮 | 通过 | 恰有一个会话项带 `active`，截图显示浅绿底加绿边 |
| 列表信息格式 | 通过 | 列表项显示 `N 条记录 · MM-DD HH:mm` |
| Trace 结构化 | 通过 | 右栏显示序号与中文标签，非原始 JSON 堆叠 |
| Trace 筛选下拉 | 通过 | `select.trace-filter` 选项为 `全部`、`仅消息`、`仅过程` |
| 标记渲染 | 通过 | 离线用例覆盖围栏代码块、行内代码、粗体、列表、标题、空行 |
| 渲染注入安全 | 通过 | 输入 `<img src=x onerror=...>` 与 `<script>` 后，渲染树中无 `img`、`script` 元素 |
| 控制台 | 通过 | 无应用报错；`/bridge/ui` 的 `ERR_ABORTED` 只在 Host 重启替换长连接时出现 |

离线用例用一个最小 DOM 替身跑通渲染器，覆盖 8 个场景，全部通过。

回归命令：

```text
go build ./...
go test ./...
node --check internal\webui\static\app.js
node --check plugins\session\ui\main.js
```

全部通过。

## 7. 第二轮：架构层交互修复

### 7.1 复核结论

第一轮 Host Shell 的改动全部落在 Host 自有界面（顶栏、面板、诊断），没有把插件业务知识塞进 Host，这条边界是守住的。

违反平台原则的地方出在 `session` 插件前端内部：

- B1：「当前选中哪个会话」这一业务状态存在浏览器里。它靠 `document` 上的自定义事件 `session-select` 传递，`session-chat` 与 `session-trace` 各自保存一份 `sessionId`。这违反 `docs/webui.md` §3「浏览器不是应用状态属主」。
- B2：B1 已经产生实际分叉。`session-history` 在 `setData` 里自动置 `selectedId` 却不派发事件，导致高亮可能与中栏正在显示的会话不一致。
- B3：选中会话时三个 Component 各自调用一次 `session.state`，而 `state` 每次都会全量扫描 JSONL，重复重读。
- B4：`store.list()` 遍历 map 不排序。map 迭代顺序随机，所以每次推送会话列表都在洗牌。
- B5：前端只在 `turn.status` 进终态时清草稿，没有按 `(turn_id, step_id)` 对账。最终 `message` 落地到 `turn_end` 之间会闪现一条重复消息。
- B6：流式每条命令都整列表重建消息，长会话必然卡顿。
- B7：Trace 每次推送整列表重建，已展开的记录会被折叠。
- B8：第一轮引入的耦合 —— 插件组件引用了 Host 的 CSS 令牌名（`--text`、`--accent` 等），把插件 UI 绑到了 Host 的内部实现上。
- B9：多标签页下推送不可用。Host 按文档只保留一条 UI 连接，`replaceUIConnection` 会停掉旧连接。旧页面的 SDK 断流后按 `reconnectDelayMs` 无条件重连，于是两个标签页反复互相抢占连接，谁都收不全推送。实测中两次浏览器验证因此失败。

### 7.2 改进点与改进方式

| 编号 | 改进点 | 改进方式 | 文件 |
|---|---|---|---|
| B1 B2 B3 | 选中会话下沉到 Runtime | 新增插件方法 `session.select`。Runtime 记录选中会话，`state` 在 `session_id` 省略时按「选中会话 → 最新会话」解析。点击历史项只发一次 `select`，由 Runtime 向三个 Mount 推 State | `plugins/session/ui_state.go`、`main.go` |
| B1 | 删除浏览器侧选择 | 去掉 `session-select` 自定义事件与 `document.addEventListener`。`session-chat` 的 `session_id` 只来自推送的 ChatState，不再由组件推导 | `plugins/session/ui/main.js` |
| B2 | 统一选中来源 | HistoryState 新增 `selected_id`，`history` Mount 只用它决定高亮。`state` 返回的 `selected_id` 与 `chat`/`trace` 的 `session_id` 同源，不可能分叉 | `ui_state.go`、`main.js` |
| B3 | 消除重复重读 | 选中只推不拉。`append` 只推 History；仅当写入的是选中会话时才推 Chat 和 Trace。`stream.update` 的 Command 也只发给选中会话 | `ui_state.go` |
| B3 | 不抢占用户视图 | `create` 不改变已有选中会话；仅当尚无选中会话时才接管，使首条消息可见。否则后台写入其他会话会抢走用户正在看的视图 | `ui_state.go` |
| B4 | 列表顺序稳定 | `list()` 显式按 `last_ts` 降序、再按 `created_ts` 降序、最后按 `session_id` 降序排序 | `plugins/session/store.go` |
| B4 | 去掉全量重读 | `store` 在内存维护 `factCount` 与 `lastTS`，读取时写入更新，`list()` 不再逐个会话读文件 | `store.go` |
| B5 | 草稿按 `(turn_id, step_id)` 对账 | `setData` 中查找 role 为 `assistant` 且 `(turn_id, step_id)` 与草稿相同的消息，命中即清草稿；终态清除作为兜底 | `main.js` |
| B6 | 流式增量渲染 | durable 列表只在 `State(ChatState)` 到达时重建；`Command("stream")` 只追加或就地更新草稿节点，内容与 reasoning 分别比对后再写 | `main.js` |
| B7 | Trace 展开态保持 | Trace 按 `seq` 缓存已渲染节点，内容未变的记录复用同一 DOM 节点。重挂载不会重置 `<details>.open`，所以推送不再折叠已展开项。换会话时清空缓存 | `main.js` |
| B8 | 去掉对 Host 令牌的依赖 | 主题变量改为插件自有的 `--session-*` 命名并全部带内置回退值，组件不引用任何 Host 令牌 | `main.js` |
| B9 | 驱逐通知 | 新连接接管时，Host 先向旧连接发送 `type: "evicted"` 的 Frame 再关闭它。SDK 收到后停止重连，并暴露 `onEvicted`。Shell 据此把状态徽标切到「已由其他标签页接管」并写入一条诊断 | `internal/webui/publisher.go`、`static/lite-agent.js`、`static/app.js`、`static/shell.css` |

B9 是平台行为补充，只新增 Frame 类型，不改既有字段，同步更新 `docs/webui.md` §7 与 §8。

`session.select`、`state` 的新解析规则、HistoryState 的 `selected_id`、推送时机与对账规则都记在 `plugins/session/communication.md`，不写入平台文档，保持「平台文档与插件文档分别维护」。

### 7.3 验证

| 验证项 | 结果 | 证据 |
|---|---|---|
| 列表顺序稳定且按时间倒序 | 通过 | 三次读取顺序完全一致，首项 `last_ts` 最新 |
| 新建会话入口 | 通过 | 点击后列表项 +1，新会话自动成为唯一 `active` 项，无报错 |
| 选中与中/右栏一致 | 通过 | 切换后 `active` 项的 `session_id` 变化，Trace 计数与该会话 `fact_count` 一致 |
| 发送无重复闪现 | 通过 | 发送前、生成中、结束后 `assistant` 消息计数均为 1，内容不重复 |
| Trace 展开态保持 | 通过 | 展开首条记录后发送消息，生成结束仍为展开 |
| 多标签页驱逐 | 通过 | 旧标签页状态变为 `evicted` 且文本为「已由其他标签页接管」，诊断计数增加 |
| 旧标签页存在时新标签页仍可用 | 通过 | 旧页仍开着时，新页点击新建会话后列表 +1、选中项稳定，静置 2 秒后数量与选中项均不变 |
| 后端新语义 | 通过 | `session.state` 省略 `session_id` 时返回选中会话的 chat/trace；`session.create` 返回新 id；`session.select` 对不存在的 id 返回 `session_not_found` |

浏览器验证在多标签页场景下失败过两次，根因即 B9；修复后同一场景通过。

### 7.4 第二轮回归

```text
node --check internal\webui\static\app.js
node --check internal\webui\static\lite-agent.js
node --check plugins\session\ui\main.js
go build ./...
go vet ./plugins/session
go test -count=1 ./...
```

全部通过。`go vet ./...` 另有一条与本次改动无关的既有告警，位于原型插件 `plugins/context-manager/main.go:33`。

## 8. 未处理项

1. 三个 Component 在页面加载时各自拉一次 `session.state`，共 3 次往返。选中与推送已不再重复拉取，但首次拉取仍是平台规定的「连接后自行取数」模式。
2. Trace 只做分页，未做虚拟列表。记录上万条时仍需优化。
3. 第二个标签页被驱逐后只提示不再接收更新，不提供「接管回来」按钮，需要刷新页面。
4. 未支持暗色模式。`shell.css` 目前固定 `color-scheme: light`。
5. `LiteAgent` 仍没有「断线中」状态回调，顶栏徽标只反映初始化、失败与驱逐三种结果。
6. 会话列表没有重命名、删除入口。

## 9. 文件清单

```text
internal/webui/static/index.html   顶栏状态、诊断关闭按钮、按钮提示、中文 aria-label
internal/webui/static/app.js       面板关闭语义、激活态、焦点、状态徽标、驱逐提示、空状态
internal/webui/static/shell.css    空槽折叠、激活态、状态徽标、驱逐与空状态样式
internal/webui/static/lite-agent.js  Evicted Frame 处理与 onEvicted 回调
internal/webui/publisher.go        驱逐通知
plugins/session/ui_state.go        Runtime 选中态、session.select、state 解析、推送时机
plugins/session/store.go           列表排序、计数缓存、created 后的发布
plugins/session/main.go            session.select 注册
plugins/session/ui/main.js         全部 session 前端交互改进
plugins/session/communication.md   插件方法、State 与前端规则
docs/webui.md                      Evicted Frame 与 SDK 行为
dist/minimal/liteagent-server.exe  重建产物
dist/minimal/plugins/session/session.exe            重建产物
dist/minimal/plugins/session/ui/main.js             同步产物
```

工作区未提交。`dist/minimal/plugins/llm-openai/config.json` 含本机 LLM 配置，不提交、不写入本文。
