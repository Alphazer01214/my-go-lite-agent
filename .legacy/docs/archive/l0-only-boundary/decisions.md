# l0-only-boundary — 开放决策共识稿

Status: locked-with-deferral（实现面已定；**#3 request/inject/confirm 明确暂缓**）  
对应：ADR-0030、`.scratch/l0-only-boundary/spec.md`

## 共识总览（2026-03 收口）

| # | 主题 | 状态 | 定案 |
|---|------|------|------|
| — | 0030 方向 | **已定** | Host/Medium L0-only；领域出核 |
| — | 启动 | **已定** | 只带 plugins；autostart+dependsOn |
| — | 展示 P2 | **已定** | Medium 可用 pluginsdk 类型 |
| 1 | static=Medium + 流式在插件 UI | **已定** | |
| 2 | Host 只认识插件、按名转发 | **已定** | 无 cap 注册表；tools 出 Host |
| Frame | v5 寻址 | **已定** | `to` + 不透明 payload；`cap`/`method` 不进 Host 分支 |
| 4 | cwd 即根 | **已定** | |
| 5 | protocol | **已定** | **升 5** |
| 6 | 金路径 | **已定** | 插件命令/UI |
| 7/7b | SDK / session_agent | **已定** | |
| 8 | 删 arch-check.sh | **已定** | |
| **3** | **request / inject / confirm** | **暂缓** | 见下；不阻塞 Z0/Z1/Z3/Z4 大部 |

---

## 暂缓 — request / inject / confirm（原 #3）

用户 2026-03：**暂时不考虑**这三者的处理。

| 影响 | 做法 |
|------|------|
| ADR-0030 | 三者在「下放表」标 **Deferred**；Z2 **先不删不改** 既有 `agent.request|inject|confirm` 处理器 |
| 与路由模型关系 | 可先做「一般 cap 路由 → 点名插件」；agent.* 横切面作为 **临时保留特例**（唯一允许的业务名分支），直到 #3 重开 |
| 重开条件 | 用户发起；届时在 A：`host.ask/tell`、B：只留 ask + 点名 session、C：其他命名 中选一 |
| 审批 UI（Z3/Z5） | sandbox/审批链路 **依赖 #3**；Z3 可先删 `/api/tool-approval` 并改为临时「通用 ask 订阅」骨架，细节等 #3 |

---

## 已锁定明细

| # | 定案 |
|---|------|
| 启动 | 只带 `-plugins` 等 L0；挂载 = autostart + dependsOn |
| 展示 | Medium 可用 pluginsdk 类型绘制（P2） |
| **1** | **static 算 Medium**：Shell 只留槽位/装载器；会话 UI 在插件 `ui/`；**流式由插件 UI 消费** |
| **2** | **Host 只认识插件**：信封 `to` + 不透明 payload；**无 cap 注册表**；tools 合并出 Host |
| **Frame** | `{ "v":5, "id", "type", "to":"<plugin>", "payload" }`；face 为 payload 内或旁路 hostFaces |
| **4** | 启动无 workspace；cwd 即根 |
| **5** | `CurrentProtocol = 5`；出厂插件升级 |
| **6** | 金路径 = 插件命令/UI；核四件装配后 CLI/Web 可聊 |
| **7/7b** | pluginsdk 可含 L1；sdk.js 收 L0；session_agent/repl 按 7b |
| **8** | 删除 arch-check.sh |

---

## 旧文保留（历史选项，勿再当待议）

### 曾待议 A — 横切面命名（**已暂缓**）

（内容见上「暂缓」节；下列选项表仅存档。）

| 方案 | ask | tell |
|------|-----|------|
| A | `host.ask` | `host.tell` |
| B | `host.ask` | 无（点名 session） |
| C | 其他命名 | |

### 曾待议 B — Protocol（**已定 A**）

`CurrentProtocol = 5`；ADR-0012 拒载纪律。

### Frame 寻址（**已定 to**）

见上表；Medium `/api/call`：`{ "to": "<plugin>", "payload": … }`。

---

## 0. 总方向

| 项 | 内容 | 状态 |
|----|------|------|
| Host 与 Medium 运行时代码不出现 session/agent/loop/llm/tools 等领域概念；领域归插件 + pluginsdk | 用户指令 | **已定** |
| supersede ADR-0002/0016 Host 侧保留，tighten ADR-0026 L1 | 写入 0030 | **已定** |
| **启动阶段只带 plugin 目录，无特定插件启动选项**；挂载真源 = `autostart` + `dependsOn` | 用户 2026-03 | **已定** |
| **Medium 允许用 pluginsdk 展示类型绘制**（RenderIntent/kind/channel）；仍禁止业务编排 API | 用户 2026-03（P2） | **已定** |
| ADR-0030 `proposed` → `accepted` | — | 待正式点头 |

---

## 1. Medium 的边界：是否包含 `web/static` 壳层 JS？

**问题**  
Go 侧 `web/server.go` 清零后，Shell 静态资源仍在编码领域：

- `sdk.js`：`LiteAgent.sendMessage` → `POST /api/message`
- `main.js`：`LiteAgent.call('session','current')`、`fetch('/api/session')`、`session-label`
- `events.js`：SSE topic `session`、`tool_approval`
- `shell.html`：`#chat` / `#session-label` 等节点语义

CONTEXT.md 对 Shell 的定义是「最薄骨架：Layout chrome + Design Token + 组件装载器」；聊天内容应属 Session View（session 插件 Panel）。当前 JS 与该定义已有漂移。

| 选项 | 含义 | 代价 |
|------|------|------|
| **A. Medium = Go + static JS 都清**（推荐） | Shell 只留槽位/装载器/Design Token；`sendMessage`、session SSE、tool_approval 监听全部删除或改为不透明 `call`/`evt` | Z3/Z5 变大；必须先有 session/agent/sandbox 真 UI |
| B. Medium 只清 Go，JS 暂留领域 | 与 ADR-0030 文字不一致，替换插件仍要改 Shell | 否决：违背「Medium 也不许出现领域词」 |
| C. JS 里的领域调用改经插件 UI 自带脚本 | Session View 组件 import 自己的 fetch；Shell 只广播不透明 evt | 与 A 同，是 A 的实现方式 |

**建议共识**：选 **A/C**。  
机械口径扩展为：`web/static/**` 不得出现 `sendMessage`、`/api/session`、`/api/message`、`/api/turn`、`/api/tool-approval`、`call('session'|'agent'|…)`；SSE 只保留 `panel`（及可选不透明 `evt`）。  
Layout **role 名**（如 `session-view`）是 **layout.json 数据**，不是 Shell 代码分支——**允许**（与 ADR-0012 一致）。若 role 字符串出现在 Shell 的 `if (role===…)` 里则禁止。

**状态**：待你确认「static JS 算 Medium」。

---

## 2. tools 多提供方：通用 multiOwner vs 独立插件

**问题**  
ADR-0030 下放表二选一，未定案。

| 选项 | Host 里还剩什么 | 优缺点 |
|------|-----------------|--------|
| **A. Manifest 通用 `multiOwner: true` + 不透明扇出**（推荐） | 对任意声明 multi 的 cap：list 时向全部 owner 请求并**原样拼接数组**（不解析字段）；call 时…仍需按名路由 → **call 无法完全不解析** | 少一跳；但「按 tool 名找 owner」仍是领域逻辑 |
| B. 独立 tools 扇出/注册插件 | Host 零 tools 知识；agent 调该插件，插件 fan-out | 多一进程；语义最干净 |
| C. 废除多提供方，tools 单 owner；其余 tools 插件改由 agent 自己消费多个 `tools` 字符串 cap 名 | Host 零特判；能力名变成 `tools.file` / `tools.shell`… | 破坏 ADR-0018；agent 要认识多个 cap 名 |

细化 A 的难点：`tools.call` 的 `{"name":"read_file"}` 必须有人解析。若 Host 只做「广播 call 给所有 tools owner，第一个认领的返回」——需要 owner 应答 `not_mine`，属通用「多 owner 认领」协议，可**去掉 name 语义**，变成：

```
multiOwner cap 的 call：按序/并行问各 owner；owner 回 `claimed` 或 `not_claimed`
```

这是 **L0 通用认领协议**，不再出现 `tools`/`name`。

**建议共识**：  
- 采用 **通用 multiOwner + claim 协议**（A′）：Manifest `multiOwner: true`；list 合并策略为「拼接 providers 返回的 raw 数组字段 `items`」（字段名在契约里固定为 `items`，Host 不解读元素）；call 为 claim 认领。  
- **不**单独拆 registry 插件（避免默认装配多一跳）。  
- `tools` 能力名只出现在插件与 pluginsdk；Host 只认 `multiOwner` 标志。

**状态**：建议采纳 A′；若你更想要零特判可改 B。

---

## 3. `agent.*` 横切面改名

| 选项 | 名字 | 语义 |
|------|------|------|
| **A（推荐）** | `host.ask` / `host.answer` / `host.tell` | ask：挂起待确认；answer：Medium/插件回；tell：只追加注入通道 |
| B | `host.confirm` / `host.notify` | confirm 易仍绑审批语义 |
| C | 下放为 session 插件方法 | 无横切面；外置 Loop 要自己找 session 插件；与 ADR-0002 进一步切割 |

**建议共识**：A。  
`host.ask` payload **完全不透明**（`json.RawMessage`）；pending 按 id；谁展示由插件 UI 订阅通用 `ask` 事件。  
原 `agent.request` 不变量改由 **session.validate** 承担，Loop 在 complete 前自调——**不再**有 Host 侧 model-context 比对。

**状态**：建议采纳 A；名字可改，但「一个 ask 槽 + tell 通道」结构建议锁定。

---

## 4. Workspace / 默认根目录

**已定（随启动模型）**：启动 argv **无** `-workspace`。进程默认根 = `os.Getwd()`，不在启动时 create session。  
「Workspace」只留在 session/filetools 等插件与 CONTEXT（L1）。Host/Medium 源码不出现该词；Web `DefaultWorkspace` 字段删除或改中性配置且不代建 session。  
插件侧 session 元数据仍可用 `workspace` 字段。

**状态**：已定。

---

## 5. Protocol / 契约版本

**问题**：`agent.request|inject|confirm` → `host.ask|tell|answer` 是插件协议破坏性变更。

| 选项 | 说明 |
|------|------|
| **A（推荐）** | `protocol` 升 5；旧 manifest 可载但旧 Host 拒新 manifest（沿 ADR-0012） |
| B | 不升版，直接切 | 旧插件静默失败 |

**建议共识**：A，与 hostFaces 转正同一纪律。

---

## 6. 金路径与 README（产品面）

**已定后果**（ADR-0030）：裸核不能 `-turn "你好"`。

**待定**：出厂体验改成什么？

| 选项 | CLI | Web |
|------|-----|-----|
| **A（推荐）** | REPL 输入即走 **session/agent 插件 commands**（如插件注册的默认命令）；或文档写「invoke loop」示例 | Session View 插件完整 UI（新建/列表/输入/停止）+ sandbox 审批卡 |
| B | 保留一个 **非领域名** 的 Host 便利入口 `-run`，内部仍 CallByCap 任意字符串 | 同 A | 与 L0 精神冲突，不推荐 |
| C | 只文档化 invoke，不做插件命令金路径 | 同 A | CLI 可用性差 |

**建议共识**：A。Z5 必须以「装配核四件后 Web 可点、CLI 可聊」为验收，不接受「只有 invoke 才能跑」。

**状态**：待确认你接受 CLI 金路径形态。

---

## 7. SDK（`pluginsdk` / `sdk.js`）边界

| 选项 | 说明 |
|------|------|
| **A（推荐）** | `pluginsdk`（Go）**可以**含 L1 常量与类型——它是插件契约包，不是 Host/Medium | 
| B | pluginsdk 也不含领域名 | 不可行，插件无法共享契约 |

`web/static/sdk.js`：  
- **保留**：`call` / `on` / `emit` / Design Token / 组件注册——L0 桥。  
- **删除或移入插件 UI**：`sendMessage`、session 专用 API。  
Panel 组件自己 `fetch('/api/call', {cap:'session',…})`——组件源码在 **plugins/**，不受 Medium 禁词约束。

**已定**：A + **P2**——`pluginsdk` 可含 L1；Medium（含 CLI paint）可 import pluginsdk 展示类型；`sdk.js` 收成 L0 桥（`call`/`on`/`emit`/DT/组件注册），删 `sendMessage` 等领域 API。

---

## 7b. `session_agent.go` / 启动面（用户定案）

| 现 CLI 启动项 | 处理 |
|---------------|------|
| `-plugins` `-repl` `-serve` `-debug` `-discover` `-dump` | **保留**（L0） |
| `-turn` `-session-append/derive/query` `-agent-request/inject` `-context-list` `-cards` `-workspace` | **删除** |
| `-invoke` `-frame-cap` `-frame-method` `-plugin` | **保留**为挂载后通用诊断（不绑领域名） |

`session_agent.go`：删领域编排；`turnRenderer` 保留并改为 `import pluginsdk` 做 paint（P2）；`cliToolApproval` 改为通用 `host.ask` 应答（不解析 tool 字段）。  
`repl.go`：去 `RunTurn`/`SessionCap`/scheme 标签；非 slash 输入由插件 commands 承接。

**状态**：已定。

---

## 8. arch-check 新项冻结清单（C13–C15 扩展）

在 spec 已有基础上冻结：

**C13（serve/ 生产 .go 禁标识符）**  
`SessionCap` `AgentCap` `LoopCap` `LLMCap` `ToolsCap` `ContextCap` `SystemPromptCap` `PresentationCap` `PolicyCap` `SkillsCap` `RunTurn` `RunTurnOn` `CancelTurnOn` `TurnCancelledOn` `AgentRequest` `AgentInject` `TurnResult` `type Message` `type ToolCall`

**C14a（web/ .go）**  
同上 + 路径子串：`/api/session` `/api/turn` `/api/message` `/api/tool-approval` `/api/workspace`

**C14b（web/static）**  
子串：`/api/session` `/api/message` `/api/turn` `/api/tool-approval` `sendMessage`（若 sdk 收窄后仍存在）

**C15**  
`serve/` 不得定义 Message/ToolCall/TurnResult。

允许：`_test.go`、`plugins/`、`pluginsdk/`、`testdata/`、构建脚本、layout.json 的 role 数据。

**建议共识**：按上表冻结进 `arch-check.sh`。

---

## 9. 已定、无需再议（防回摆）

- `host.ensurePlugins`、hostFaces（config/commands/ui）、PanelOp 组件前缀校验保留。
- 插件目录名字面量禁令（C1）不回退。
- 测试仍进程边界集成，不 mock 化提速。
- 调试面**展示** Manifest 能力名允许；**代码特判**不允许。
- 不变量目标「模型可见即已记录」不变，只是实现离开 Host。

---

## 建议的确认方式

请直接回复一段即可，例如：

```text
0030 accepted
1A  2A'  3A  4A  5A  6A  7A  8冻结
```

或指出要改的编号（如「2 改 B」「6 保留 -run」）。  
确认后：ADR-0030 翻 accepted 并写入决策表；spec Z0–Z5 按定案微调；再开工实现。

---

## 决策一览（供勾选）

| # | 主题 | 状态 | 定案 |
|---|------|------|------|
| 0 | 0030 accepted | 待点头 | 方向已执行于文档 |
| 0b | 启动只带 plugins，无插件专用启动项 | **已定** | autostart+dependsOn |
| 0c | Medium 可用 pluginsdk 展示类型 | **已定** | P2 |
| 1 | static JS 算 Medium 并清领域 | 建议 A/C | |
| 2 | tools 多 owner | 建议 A′ claim | |
| 3 | ask/answer/tell | 建议 A | |
| 4 | 启动无 workspace；cwd 即根 | **已定** | |
| 5 | protocol 5 | 建议 A | |
| 6 | CLI/Web 金路径 | 建议 A（插件命令） | |
| 7 | pluginsdk L1 + sdk.js L0 桥 | **已定** | 含 P2 |
| 7b | session_agent/repl 拆法 | **已定** | 见 7b |
| 8 | C13–C15 词表 | 建议冻结 | |
