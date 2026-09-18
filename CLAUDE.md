# my-go-lite-agent — Agent 约束

轻量 Go Agent 运行时：**Host 薄内核 + 进程外插件**。核心零第三方（解析库除外）；拿到二进制与插件目录即可运行。

## 理念（不可回退）

- **一切皆插件**：Session / LLM / Agent / 工具 / 上下文 / 策略皆可发现、可组装、可替换。
- **日志是真源**：Session Log 只追加；模型可见内容必须能从日志重建。
- **轻**：进程隔离换崩溃边界与独立分发；不绑重框架，不堆 harness。

规范名词以 [CONTEXT.md](CONTEXT.md) 为准；架构决策以 [docs/adr/](docs/adr/) 为准。输出中使用规范名词，避免同义漂移。

## 必读顺序

| 触发场景 | 打开 |
|----------|------|
| 改 Host / Medium / 路由 / 启动面 | [ADR-0030](docs/adr/0030-l0-only-host-and-medium.md) → [ADR-0026](docs/adr/0026-host-plugin-boundary-criteria.md) → [ADR-0027](docs/adr/0027-plugin-host-faces.md) → [docs/modules/](docs/modules/) |
| 改插件 / manifest / UI 面 | [plugins/README.md](plugins/README.md) → [ADR-0027](docs/adr/0027-plugin-host-faces.md) → [ADR-0012](docs/adr/0012-web-ui-contract-v2.md) → [ADR-0031](docs/adr/0031-shell-five-regions.md) |
| 改装配 / scheme / 挂载 | [ADR-0021](docs/adr/0021-autostart-depends-on-over-assembly.md) → [ADR-0023](docs/adr/0023-host-ensure-plugins.md) → [ADR-0022](docs/adr/0022-consumes-soft-skip-degraded.md) |
| 改 Agent Loop / 会话 / 压缩 | [ADR-0016](docs/adr/0016-agent-plugin-no-host-loop.md) → [ADR-0028](docs/adr/0028-auto-compact-as-default.md) → [ADR-0013](docs/adr/0013-context-prepare-and-log-native-compaction.md) |
| 改策略 / 审批 / 工具面 | [ADR-0019](docs/adr/0019-policy-capability-not-interceptor.md) → [ADR-0018](docs/adr/0018-tools-capability-multi-provider.md) |
| 查非插件模块职责 / API | [docs/modules/README.md](docs/modules/README.md) |
| 写票据 / 开 feature | [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md) → [docs/agents/triage-labels.md](docs/agents/triage-labels.md) |
| 术语与 ADR 冲突 | [CONTEXT.md](CONTEXT.md) → [docs/agents/domain.md](docs/agents/domain.md) |

历史 spec 已归档于 [docs/archive/](docs/archive/)，**不是现行契约**。

## 架构红线

违反任一条即停手，标注出处并询问；不得静默覆盖 ADR。

1. **Host / Medium 只保留 L0**（ADR-0030）  
   `serve/`、`web/`（含 `web/static`）、`internal/app/` 运行时面仅做：按**插件名**的 Frame 转发（`to` + 不透明 payload）、进程生命周期、通用挂载、通用事件/应答、`hostFaces`、`host.ensurePlugins`。  
   **不得**将 `session` / `agent` / `loop` / `llm` / `tools` / `context` / `policy` 等领域名作为分支条件、专用 API 或领域 HTTP 面。领域语义只存在于插件与 `pluginsdk`。

2. **Host 不解读领域 payload**（ADR-0026）  
   转发层持 `json.RawMessage`；不为插件 payload 定义字段级结构体做业务解析。构造 L1 / hostFace **请求体**属于消费公开契约，允许。

3. **Host 不写死插件目录名做调度**（ADR-0026 / 0030）  
   `serve/`、`web/`、`internal/` 生产源不出现 `plugins/` 下目录名字面量。调用方点名目标插件是 L0 寻址，不是 Host 特化。

4. **Agent 是插件，Host 无内建 Loop**（ADR-0016 / 0030）  
   不把 `loop.turn` 编排、per-session 锁、Cancel 放回 Host。  
   **Deferred 例外**：`agent.request` / `agent.inject` / `agent.confirm` 暂由 Host 特例处理；重开前勿扩大该面，勿据此恢复 cap 路由。

5. **装配真源 = Autostart + dependsOn 闭包**（ADR-0021）  
   日常挂载不走 Assembly 白名单。`-assembly` 仅 deprecated 调试对照。场景工具由 Agent Scheme `dependsPlugins` 经 `host.ensurePlugins` 拉起。

6. **软失败，进程继续**（ADR-0017）  
   Discovery / Assembly / 单插件启动失败：错误可见，跳过该项，Host 不退出。用到缺失能力时再报错。

7. **策略是 Capability，不是路由中间件**（ADR-0019）  
   不恢复 Waterfall / Interceptor。Agent Loop 在 callTool 前主动 `policy.decide`；Host 只保留 closed 拒绝（ADR-0014）。

8. **插件保持进程外**（ADR-0001）  
   不改回同进程 / `.so` 热插拔。跨插件调用经 Host，插件之间不直连。

## 插件开发约定

- `plugin.json` 是元数据**唯一真源**：`name` / `version` / `protocol` / `provides` / `consumes` / `entry` / `timeoutMs` / `commands` / `ui` / `autostart` / `dependsOn` / `hostFaces`。构建脚本只编译与拷贝，不生成 manifest。
- 插件作者经 `pluginsdk` 说话（Handle / Call / Emit / Serve）；夹具可手写 Frame，生产代码不手写 Frame 循环。
- `hostFaces` 仅 `config` | `commands` | `ui`（ADR-0027）。这三面按**插件名**寻址，与 `provides`（能力名、唯一属主）语义不同，不得混入 `provides`。
- `provides` / `consumes` 是声明与观测（degraded、Plugin Graph）；运行时调用点名插件或由插件自建发现。
- Panel Component 元素名以插件名为前缀（`<plugin>-*`）；样式走 `--la-*` Design Token；交互经 SDK 回插件。
- `protocol` 字段 ∈ `1..plugin.CurrentProtocol`（当前 **6**）。破坏 Manifest/UI 契约 → bump `CurrentProtocol` 并升级出厂插件；破坏 Frame 线格式 → bump `protocol.Version`（当前 **5**）。
- 核心与 Host 零第三方依赖（解析库除外）；插件优先 stdlib。

## 变更纪律

- **一个逻辑单元一个 commit**；禁止 `--no-verify`。
- **破坏性变更必须有 ADR 出处**。与现有 ADR 冲突时：停下、在输出中标注冲突点、询问；不静默覆盖、不擅自 supersede。
- 新决策：先更新/新增 ADR，再改代码；实现与 ADR 不一致时以代码评审对照 ADR 边界表（不再使用 `arch-check.sh`）。
- 测试以进程边界集成测试为主（现场 `go build` 夹具插件）；不为提速改成 mock。
- 每个逻辑单元结束：`go build ./... && go vet ./... && go test ./...` 全绿。

## 协作流程

- Issue / spec 落在 `.scratch/<feature-slug>/`（spec + `issues/NN-*.md`）。约定见 [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md)；状态标签见 [docs/agents/triage-labels.md](docs/agents/triage-labels.md)。
- 已完结 feature 归档到 `docs/archive/`，只留未完结在 `.scratch/`。
- 输出使用 [CONTEXT.md](CONTEXT.md) 规范名词；与 ADR 冲突显式标注（见 [docs/agents/domain.md](docs/agents/domain.md)）。

## 现行 ADR 结论速查

只列**仍约束当前实现**的结论；全文与 supersede 关系见各 ADR。

| ADR | 绑定结论 |
|-----|----------|
| 0001 | 进程外插件；跨插件调用经 Host，插件不直连 |
| 0005 | 失败/取消不写半截 assistant |
| 0007 | 主窗内容三类：`markdown_text` \| `message_text` \| `summary_text` |
| 0008 | 原生 slash 仅 `/help` `/lp` `/refresh` `/exit`；其余归插件 commands |
| 0010 | Panel 内容 = Panel Component；PanelOp `set`/`clear`；组件前缀校验保留 |
| 0012 | 磁盘 Layout 真源 + 插件加法贡献 + Assembly 裁决；protocol 3 破坏性 UI 契约 |
| 0013 | Compact 以日志事实落地，不改写 Session Log |
| 0014 | 删除 Waterfall / Interceptor / 审计链 |
| 0015 | Tool Result Stub、Context Window 观测、`compactHint` |
| 0017 | 装配与运行时软失败 |
| 0018 | 多 `tools` 提供方可共存；**合并已出 Host**（0030 收紧），由 loop/agent 按名 fan-out |
| 0019 | Permission = `policy` Capability；Loop 主动 decide |
| 0020 | Workspace 是 Session 元数据；filetools 等只认此根 |
| 0021 | Autostart + dependsOn 为日常挂载真源 |
| 0022 | consumes 缺失 → degraded，软跳过 |
| 0023 | `host.ensurePlugins` 幂等按名挂载 |
| 0024 | Status Bar / New-session Face 归 Layout 槽位与插件 UI |
| 0025 | Plugin Graph 读 Discovery 全量，节点带 mounted/available/degraded/missing |
| 0026 | L0/L1/L2 分层与三条机械规则（被 0030 收紧 L1） |
| 0027 | `hostFaces` 转正；导出面收窄；无 `-scheme` |
| 0028 | 自动压缩是默认行为 |
| 0029 | Medium 展示钩子为订阅列表，非单槽 |
| 0030 | Host/Medium L0-only；按插件名路由；`CurrentProtocol = 5`（被 0031 升到 6 的槽位面另见）；`agent.*` Deferred |
| 0031 | Shell 五块通用区域 `top\|bottom\|left\|center\|right`；session 占左+中（rail / chat+trace）；`CurrentProtocol = 6` |

**明确不做**：同进程插件、Waterfall/Interceptor、Host 内建 Loop、Assembly 白名单作日常真源、Host 合并 tools、Medium/CLI 领域启动 flag 与领域 HTTP 面、恢复 cap 注册表路由。
