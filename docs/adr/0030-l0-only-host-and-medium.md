# Host 与 Medium 收紧为 L0-only：领域概念全部出核

**Status: accepted**

## Context

ADR-0026 允许 Host 作为 L1 公开能力（`session` / `llm` / `loop` / `tools` / `context`…）的**消费者**；ADR-0002 / 0016 进一步把 Session Log 不变量、`loop.turn` 编排、per-session 锁与 `agent.request`/`agent.inject` 留在 Host。CLI/Web Medium 则直接以这些能力名拼 HTTP 面与诊断 flag。

复审结论（2026-03）：生产代码已无插件**目录名**字面量（C1），但 Host/Medium 仍深度编码领域概念——`serve` 解析 `Message`/`TurnResult`、特判 `tools` 多提供方、实现 `AgentRequest`；`web` 暴露 `/api/session/*`、`/api/turn/cancel`；`internal/app` 的 `-turn`/`-session-*`/`-agent-*` 与 `session_agent.go` 同理。

用户裁决：Host 必须**真正通用**；且 **CLI/Web Medium 同样不得出现** `session` / `agent` / `loop` / `tools` 等领域词。这比 ADR-0026 更严，属有意收紧，不是回退去特化成果。

## Decision

**Host（`serve/`）与 Medium（`web/`、`internal/app/` 中的运行时面，含 `web/static`）只保留 L0：按**插件名**寻址的 Frame 转发、进程生命周期、通用挂载、通用事件/应答总线。一切领域语义（会话、回合、模型消息、工具、策略、上下文、展示分类）只存在于插件与 `pluginsdk` 契约包。**

### 路由模型（用户定案 2026-03）

**Host 只认识插件，不认识能力名。**

| | 旧（ADR-0001 星型 + cap 注册表） | 新（0030） |
|--|----------------------------------|------------|
| 寻址键 | `provides["session"]` → owner | Frame/请求里的 **目标插件名** |
| Host 维护 | capability → plugin 注册表、tools 合并 | 仅 plugin name → 进程（生命周期） |
| 转发语义 | 解析 `cap`/`method` 决定去哪 | 调用方在 payload/信封中写明 **目标插件** 与参数；Host **原样转发** |
| 能力名 | Host 路由真源 | 仅 Manifest/Discovery **数据**（图、插件间解析）；Host **不**按 cap 分支 |

示意：

```text
插件 A ──Frame{to: "session", payload: {...}}──► Host ──转发──► 插件 session
                （Host 不解读 payload）
```

- `consumes`/`provides` 保留为 **声明与观测**（degraded、Plugin Graph）；运行时调用改为 **点名插件** 或由插件自建发现（读 `/api/plugins`、config、scheme `dependsPlugins` 等）。
- `tools` 多提供方 **不再**由 Host 合并：由 loop/agent 插件按 scheme 拉起的插件名自行 fan-out，或独立聚合插件；Host 零 tools 语义。
- `hostFaces`（config/commands/ui）与 `host.ensurePlugins` 本来就是按插件名，与本模型一致。
- 调试面**展示** Manifest 能力名允许；Host **代码**不得 `switch cap` 到业务名。

### 分层（替换 ADR-0026 的 L1「Host 可消费」）

| 层 | 内容 | Host | Medium |
|----|------|------|--------|
| **L0** | 按插件名转发、fwd-id、pending、超时、进程生命周期、`host.ensurePlugins`、`hostFaces`、通用 pub/sub、通用 ask/answer 槽 | 应拥有 | 只经 Host 使用 |
| **L1** | 能力名与方法、payload 领域字段 | **不得调用、不得解析、不得出现常量名** | **Go 运行时不得出现**；绘制可用 pluginsdk 展示类型（下文） |
| **L2** | 插件**目录名**写死在 Host（调度硬编码） | 绝不能知道（调用方点名目标插件是 L0 寻址，不是 Host 写死名字） | 同理 |

「不得出现」的机械口径（**不再依赖 arch-check.sh**，该脚本删除；以代码评审 + 测试约束）：`serve/`、`web/`、`web/static/`、`internal/app/` 生产源不得出现业务能力编排 API 与领域 HTTP 面——`session`/`agent`/`loop`/`llm`/`tools`/`context`/`policy` 等作为 **Host/Medium 分支条件或专用入口**；`RunTurn`、`AgentRequest`、`AgentInject`、`/api/session` 等。  
**允许**：Medium `import pluginsdk` 做展示绘制；插件源码与 pluginsdk 含全部 L1；测试与构建脚本。

### 下放表（概念迁往何处）

| 现 Host/Medium 概念 | 迁往 | 说明 |
|---------------------|------|------|
| `AgentRequest` 不变量（ADR-0002） | **session 插件** `validate` 或 Medium 侧插件 UI 调 derive 后自比 | 不变量仍在，但不在星型内核；ADR-0002 被本 ADR **supersedes** |
| `RunTurn` / `turnStates` / `CancelTurn*` | **loop 插件**自持锁与 cancel；入口为 `loop.turn` / `loop.cancel` | Medium 经 `/api/call` 或 Panel UI Action 调用；ADR-0016 中「Host 持锁」**supersedes** |
| `Message` / `ToolCall` / `TurnResult` | **pluginsdk**（或 session/loop 契约） | Host/Medium 只 `json.RawMessage` |
| `agent.request` / `agent.inject` / `agent.confirm` | **Deferred（用户 2026-03 暂缓）** | 先保留现行为；点名路由落地后另议 ask/tell 或点名 session |
| `tools` 多提供方合并 | **Host 删除**；loop/agent 按已挂载插件名 fan-out，或独立聚合插件 | Host 零 tools 语义（定案） |
| Presentation 方法名特判 | 只透传 `TypeEvt`；流式/绘制契约在 **pluginsdk**；**Session View 等插件 UI 负责消费流式**（start/chunk/end、channel），Shell 不碰 | Medium 可用 pluginsdk 类型绘制（P2） |
| capability 名路由 | 见上文「路由模型」；`CallByCap` 若保留则为 Medium/插件侧解析后的点名转发，**Host 内核无 cap 注册表分支** | |
| `hostUsedCapabilities` 写死名单 | 删除；图边来自 Manifest/点名关系 | |
| Web `/api/session/*`、`/api/turn/cancel`、`/api/message`、`/api/tool-approval`、`/api/workspace/resolve` | **删除**；会话 UI 在插件 `ui/`；Shell 只留槽位/装载器/SDK 桥 | |
| CLI `-turn`/`-session-*`/`-agent-*`/`session_agent.go` 领域路径 | 见启动模型；REPL 聊天由插件 commands 承接 | |
| Web `currentSession` 缓存 | 删除；Current 由 session 插件能力自持（CONTEXT.md 已如此定义） | |

### 启动模型（用户定案 2026-03）

**启动阶段只携带插件目录，不携带任何特定插件的启动选项。**

| 允许（L0 基础设施） | 禁止（领域/插件专用启动项） |
|--------------------|---------------------------|
| `-plugins`（Discovery 根） | `-turn`、`-session-*`、`-agent-*`、`-context-list`、`-cards` |
| `-repl` / `-serve`（Medium 模式） | `-workspace` 作为 session 绑定（改为进程 cwd / 通用配置，不代 create） |
| `-debug`、`-discover`、`-dump` | `-scheme`（已删，ADR-0027） |
| 挂载真源 = Manifest `autostart` + `dependsOn` 闭包（ADR-0021） | CLI 启动时点名拉起某插件、预选 scheme、代建 session |

启动后经 **通用** 面交互：`/api/call`、hostFaces、commands、Panel UI Action、以及可选的通用 `-invoke`/`-frame-cap`（诊断，不绑领域名）。  
领域动作（发消息、切 scheme、append 事实）一律由 **已挂载插件** 的 command/UI 承接，不进进程 argv。

### Medium 与 pluginsdk 展示契约（用户定案 2026-03）

Medium **允许** `import pluginsdk` 使用展示类型做绘制（`RenderIntent`、`SummaryPair`、stream channel、markdown/message/summary kind）——这是 **Render Medium 底座**，不是对业务能力的消费。  
Medium **仍禁止**：在 Go/JS 里编排 session/loop 等业务、启动 argv 编码领域行为。  
**流式（定案）**：Session View 等 **插件 Panel** 消费 stream（start/chunk/end、content/reasoning）；Shell 只装载组件与透传 evt，**不**在 `web/static` 里拼聊天流状态机。CLI `turnRenderer` 可保留分 kind 绘制（依赖 pluginsdk）。

### 验收方式（用户定案 2026-03）

- **删除** `scripts/arch-check.sh`（不再作机械闸门）。  
- 验收 = `go build ./... && go test ./...` 全绿 + 金路径集成测试 + 对本 ADR 边界表的代码评审。  
- 原 C1–C15 检查项降为 **评审清单**（写在 spec，不进脚本）。

### 保留（有意不删）

- `host.ensurePlugins`（ADR-0023）：按插件名幂等挂载，无领域语义。
- `hostFaces` 派发与 `/refresh` 的 `config.reload` 广播（ADR-0027）。
- PanelOp 组件前缀校验（ADR-0010）。
- 进程默认根目录：`os.Getwd()`，**不**叫 workspace、**不**在启动时 create session。
- 构建/Discovery/Assembly 文件与插件名出现在 **build 脚本与测试** 中，不受本 ADR 约束。

## Consequences

- 替换 session/loop/sandbox 不需要改 Host 或 Medium 源码——符合「一切皆插件」终态。
- 默认金路径（新建会话 → 输入 → 回合 → 审批）**全部依赖出厂插件 UI**；裸 Host+空 Medium 无法聊天。与 ADR-0017 软失败一致：无插件时装配可见错误，进程仍起。
- ADR-0002 的「不变量在星型中心」被放弃：外置 Loop 若不调 session.validate 则不变量可被绕过。用金路径测试 + agent 插件文档约束（同 ADR-0019 对 policy 的处理）。
- 一次性 `-turn "你好"` 在 CLI 上不再是 Host 内建；启动 argv 只有 `-plugins` 等 L0 项；金路径靠 Autostart 挂载后的插件 command/REPL。README 快速开始全部改写。
- Medium 绘制可继续用 pluginsdk 展示类型；流式状态机在插件 UI。  
- Host 无 cap 注册表后，插件点名调用依赖 scheme/`dependsOn`/发现数据；默认装配仍靠 autostart+dependsOn。  
- **`protocol` 升 5**（定案）：Frame 寻址 `to` + 不透明 payload；出厂插件升级；旧 Host 拒新 manifest（ADR-0012）。  
- **`agent.request` / `agent.inject` / `agent.confirm`：暂缓改造（Deferred）**——Z2 可先做点名路由，这三者作为临时保留特例，命名与去向另开决策后再动。  
- 行为破坏性来自 Medium API 面、启动面与 Host 路由模型收缩，属 major 产品面变更。
- arch-check 脚本删除；边界以本 ADR + 评审 + 测试约束（不再增补 C13–C15 脚本项）。

## Supersedes / contradicts

| 文档 | 冲突点 | 处理 |
|------|--------|------|
| ADR-0002 | 不变量放 Host | **Supersedes** 不变量实现位置；「模型可见即已记录」目标不变 |
| ADR-0016 | Host 保留锁/Cancel/request/inject | **Supersedes** Host 侧清单；Agent 仍为插件 |
| ADR-0026 | L1 Host 可消费 | **Tightens**：L1 对 Host 与 Medium 同时禁止 |
| ADR-0023 / 0027 / 0010 / 0021 | — | 继续有效 |
| CONTEXT.md 中「Host … Session 不变量、agent.request/inject」「Session View 由 Host 路由 turn」等 | 术语需随实现修订 | 实现 Zone 内更新，不在本 ADR  silently 改 glossary |

## Alternatives considered

1. **维持 ADR-0026 L1**：Host 可当 session/loop 消费者。否决——用户明确要求核内零领域概念。
2. **只清 serve/，Medium 可留 L1**：改动小。否决——Medium 同样写死领域 API，替换插件仍要改 HTTP 面。
3. **保留 `agent.*` 横切面改名不删**：折中。采纳改名为 `host.ask`/`host.tell`，避免「agent」一词残留。
