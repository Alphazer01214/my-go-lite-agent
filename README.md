# my-go-lite-agent

轻量 **Go Agent 运行时**：Host 薄内核 + 进程外插件。拿到二进制和插件目录就能跑，不要求读源码；核心零第三方依赖。

```text
用户输入
   │
   ▼
┌──────────── Host（L0 薄内核）─────────┐
│  Discovery · Assembly · 生命周期       │
│  按插件名转发（Frame To + 不透明载荷） │
│  通用事件/应答 · hostFaces · ensure    │
└───┬──────────┬──────────┬──────────┬──┘
    │          │          │          │
 session     llm      context-manager tools
（插件）  （插件）    （插件）      （插件）
    ▲          ▲          ▲          ▲
    └──────────┴────┬─────┴──────────┘
                    │
                 agent（插件，provides loop）
```

Host 不认识能力名，不解析领域 payload；会话、回合、工具编排全在插件（ADR-0030）。

## 理念

- **一切皆插件**：Session、LLM、Agent、工具、上下文观测都是可发现、可组装、可替换的进程。
- **日志是真源**：会话事实只追加；模型能看到的内容必须能从 Session Log 重建。
- **轻**：进程隔离换崩溃边界与独立分发；不绑重框架，不堆臃肿 harness。

规范名词见 [CONTEXT.md](CONTEXT.md)；架构决策见 [docs/adr/](docs/adr/)；模块开发文档见 [docs/modules/](docs/modules/)。

## 特性

- 进程外插件（stdin/stdout JSON Frame），崩溃隔离
- **Autostart + dependsOn**：Manifest 根集 + 依赖闭包；不再依赖日常 Assembly 白名单（ADR-0021）
- **L0 转发**：Host 按插件名寻址（Frame `to` + 不透明 payload），不按能力名分支（ADR-0030）
- Session Log 不变量 + Agent 插件（提供 `loop`，自行组合 llm/session/tools）
- **Agent Scheme**：`chat` / `tool_calling` / `coding`（config 可自定义）；`dependsPlugins` + `allowedTools`
- Context Manager：System Prompt 组装、上下文占用、查看进入模型的 messages
- `llm-openai`：OpenAI 兼容（DeepSeek 等），流式输出
- CLI REPL / Web Shell（新建会话面 + 聊天 + 中心 Session Trace + 底栏信息区）
- **Workspace**：Session 级项目根（CLI 默认 cwd，Web 会话可选）
- **coding Scheme**：filetools + shelltools + sandbox + skill-manager + project-context + webtools
- **Tool severity sandbox**：工具 schema 声明 `severity`（low/medium/high），sandbox 按 `severityPolicy` 拦截（默认 medium/high→ask）；显式 rules 仍优先
- 多 `tools` 插件共存；`$skill` 输入触发；Todo / Plan Constraint（提示约束）
- **Plugin Graph**：`Plugins` 弹窗按 Discovery 全量目录画依赖图，节点标 mounted / available / degraded（ADR-0025）
- 跨平台：Windows / macOS / Linux

## 快速开始

### 构建

```powershell
# Windows — 增量（默认）：按源码哈希跳过未变更的二进制
.\scripts\build.ps1

# 只重建某个目标（短名即可）
.\scripts\build.ps1 -Only sandbox
.\scripts\build.ps1 -Only liteagent-server,sandbox

# 全量：清空 dist 后重编
.\scripts\build.ps1 -Clean

# 列出可构建目标
.\scripts\build.ps1 -List
```

```bash
# macOS / Linux
bash scripts/build.sh
bash scripts/build.sh --only sandbox
bash scripts/build.sh --clean
bash scripts/build.sh --list
```

增量模式会对每个目标（cli / server / 各插件）做内容哈希（模块内依赖 `.go` + `go.mod`/`go.sum`；server 另含 `web/static`）。哈希未变且 `dist` 里已有产物时跳过 `go build`；`plugin.json` / `ui/` / README / examples 等资产始终刷新。缓存戳记在 `dist/.build-cache/`。

产物在 `dist/`：`liteagent-cli`、`liteagent-server`、`plugins/`、`examples/`。

### 配置模型

```powershell
# Windows（优先环境变量）
$env:OPENAI_API_KEY = "sk-..."
$env:OPENAI_BASE_URL = "https://api.deepseek.com/v1"   # 默认 DeepSeek
$env:OPENAI_MODEL = "deepseek-chat"
```

```bash
# macOS / Linux
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.deepseek.com/v1
export OPENAI_MODEL=deepseek-chat
```

也可编辑 `dist/plugins/llm-openai/config.json`，或在 REPL / Web 输入框里 `/llm-openai config set apiKey=sk-...`。

**构建不会动你的密钥**：`config.json` 不进 git；重新构建时，dist 里已有的配置原样保留（首次构建才会从仓库根的 `plugins/llm-openai/config.json` 播种）。

### 跑起来

启动只带 L0 项：`-plugins` / `-repl` / `-serve` / `-debug` / `-discover` / `-dump`（ADR-0030）。领域动作由已挂载插件的 command / UI 承接。

```powershell
# 金路径：REPL 直接输入即走 agent 插件 loop.turn
.\dist\liteagent-cli.exe -plugins dist\plugins -repl

# Web
.\dist\liteagent-server.exe -plugins dist\plugins -serve 127.0.0.1:8080

# 通用点名诊断（不绑领域名）
.\dist\liteagent-cli.exe -plugins dist\plugins -invoke agent -frame-cap loop -frame-method turn -invoke-payload '{"input":"你好"}'
```

```bash
./dist/liteagent-cli -plugins dist/plugins -repl
./dist/liteagent-server -plugins dist/plugins -serve 127.0.0.1:8080
```

**Agent Scheme**（`dist/plugins/agent/config.json`，可用 `/agent config set defaultScheme=coding` 热切换）：

| scheme | 含义 |
|--------|------|
| `chat` | `dependsPlugins` 拉起 filetools；显式 `allowedTools` 只读名单（read_file/grep/glob），无 subagent、无 todo |
| `tool_calling` | 默认完全体，不按名单过滤 |
| `coding` | `dependsPlugins` 拉起 filetools/shelltools/sandbox/skill-manager/project-context/webtools |

`-assembly` 仍可传入但已 **deprecated**（仅测试/对照；日常不要用装配白名单）。

Host 调试日志：加 `-debug` 后，Host 边界上的 Frame（方向 / 插件 / id / cap / method / payload）与插件启停会打到 **stderr**，不污染 stdout 的对话输出。

Coding Scheme：先设 `defaultScheme=coding`（`dist/plugins/agent/config.json` 或 REPL 里 `/agent config set defaultScheme=coding`），进 REPL 对话；工作区可放 `.liteagent/permissions.json`、`.liteagent/skills/<name>/SKILL.md`、`AGENTS.md`。

## 常用命令

**REPL / Web 输入框**

| 命令 | 说明 |
|------|------|
| `/help` | 帮助 |
| `/lp` | 已挂载插件 |
| `/session list` / `derive` / `dump-trace` | 会话列表 / Model Context / 导出日志 |
| `/context-manager usage` / `list` | 上下文占用 / 进入模型的 messages |
| `/llm-openai config` | 模型配置 |
| `/agent config` | Agent Scheme（defaultScheme） |

原生 slash 仅 `/help` `/lp` `/refresh` `/exit`；其余能力在对应插件名下。

**Web 界面**

- **新建会话面**：打开页面停在「工作区选取（可留空）+ agent 模式 + 聊天框」，**不会**自动加载当前会话。目录选择后需手填绝对路径（Medium 不再解析目录名，ADR-0030）。发出第一条消息时才创建会话并绑定该工作区。左侧会话列表**按工作区路径分组**，点一下即进入历史会话。
- **底栏信息区**：Shell 区域 `bottom`（ADR-0031），内容由各插件自己的 `<plugin>-status` 组件提供——工作区 / 会话 / 事实数（session）、token 与窗口占比（context-manager）、模型名与模型总时长（llm-openai）、当前 scheme（agent）。session 占 `left`（会话列表）与 `center`（chat|trace）。
- **Settings**：打开插件设置浮层。每个实现了 `config.schema`/`config.get` 的插件会出现在左侧列表，右侧按该插件自己的 schema 渲染表单；保存走 `config.set`（热生效）。插件也可注册自定义元素 `<plugin-name>-settings` 完全接管该面板。
- **Plugins**：插件 / Capability / Host / UI 槽位的关系图。实线绿=已挂载，虚线灰=已发现但待拉起（由某个 Agent Scheme 的 `dependsPlugins` 决定），红=degraded（consumes 未满足），琥珀=dependsOn 引用了未发现的插件。

**CLI 诊断（L0）**

```powershell
-invoke PLUGIN     点名调用插件（诊断）
-frame-cap CAP     Frame cap（插件侧 dispatch key）
-frame-method M    Frame method
-invoke-payload J  JSON payload
-debug             Host Frame 调试日志（stderr）
```

产品启动**没有** `-turn` / `-session-*` / `-agent-*` / `-workspace` / `-context-list` / `-cards`（ADR-0030）。集成测试可在 `L0_TEST_COMPAT=1` 下使用隐藏兼容 flag。

## 装配示例

日常 **不需要** assembly 文件。挂载集 = Manifest `autostart` 根 + `dependsOn` 闭包；场景工具由 Agent Scheme `dependsPlugins` 经 `host.ensurePlugins` 拉起。

`dist/examples/*.json` 仅作 **reference-only**（deprecated `-assembly` 白名单，测试/对照用）。

## 文档

- 插件索引（含状态标记：核四件 / 场景工具）：[plugins/README.md](plugins/README.md)
- 术语与边界：[CONTEXT.md](CONTEXT.md)
- 架构决策：[docs/adr/](docs/adr/)
- 模块开发文档：[docs/modules/](docs/modules/)
- 线协议总览：[docs/protocol.md](docs/protocol.md)
- 历史 spec 归档：[docs/archive/](docs/archive/)（开发过程稿，非现行契约）
