# CLI Render & Commands（主窗口渲染 + 插件命令）

Status: ready-for-agent

## Problem Statement

Host CLI 已有基础三分类渲染（markdown / expandable / message）与 `turnRenderer`，但：

1. `mdansi` 只覆盖 Markdown 子集，表格/嵌套列表/链接均缺失，对齐 Claude Code 体验不够。
2. 工具调用展示为「`⏺ name` + 缩进 JSON」+ 结果 expandable，信息过载，缺少摘要式一行卡。
3. 没有 slash command 体系：不能 `/help`、不能列插件、不能在主窗口配置 llm apikey。
4. Presentation taxonomy（`markdown|expandable|message`）与目标形态（`markdown_text|message_text|summary_text`）不一致，需一次 wire 级整理。

## Solution

### 1. Presentation taxonomy 重写（protocol v2）

三类渲染意图替换为：

| Kind | 职责 | 谁发 |
|---|---|---|
| `markdown_text` | 正文 Markdown | 插件 / Host settle |
| `message_text` | 短状态行（thinking、running…） | Host / 插件 |
| `summary_text` | 键值对摘要（工具卡等） | Host（工具路径）/ 插件 |

- `expandable` 退役；其「摘要 + 详情」语义由 `summary_text` 的 `title/pairs/detail` 承担。
- `Card` 路径（`presentation.card`）不变。
- `protocol.Version` → **2**；manifest `protocol` 要求 2；protocol=1 Discovery 警告仍尝试挂载，不识别的 render kind 丢弃并 warn。
- SDK 更名：`EmitMarkdownText` / `EmitMessageText` / `EmitSummaryText`；删除旧方法，不留 alias。

**summary_text 载荷：**

```json
{
  "title": "read_file",
  "pairs": [{"key": "path", "value": "a.go"}],
  "detail": "optional longer body"
}
```

- `pairs` 为保序数组。
- 截断（rune 计）：`value` 120、`pairs` 最多 8 条（余下并入 detail）、`detail` 400；超出加 `…`。
- 插件负责预截断；Render Medium 做硬上限兜底。
- 截断只影响终端显示；Session Log 中 `tool_result` 仍存全文。

### 2. Markdown 渲染：goldmark + 自写 ANSI 适配

- 引入 `github.com/yuin/goldmark` 作 CommonMark 解析（符合「解析库除外」）。
- `render/mdansi` 保留为 ANSI 输出层：goldmark AST → ANSI；删除手写行扫描解析。
- 风格沿用现有 token（标题 cyan、代码围栏 dim 边框、引用 gray `│` 等），按需扩展表格/链接/嵌套列表。
- 不引入 glamour/charm 栈。

### 3. 工具调用渲染时序（Host 统一发）

```
CallTool 开始  → message_text  "Running read_file…"
CallTool 结束  → summary_text  title=tool, pairs=关键 args（截断）, detail=结果摘要（截断）
```

- 无 Presentation 面的工具也有默认像样渲染。
- 工具插件仍可选 `EmitCard` 补充自定义卡。
- Thinking：短状态用 `message_text` level=`dim`（CLI ANSI dim，无图标）；长思考流仍走 `presentation.stream`（ephemeral，不入 Session Log）。
- `message_text` level 扩展：`info | warn | error | dim`。

### 4. Settle 与 markdown_text

- 流式过程：单行 dim 进度 `Generating… N chars`（不把 raw delta 当正文）。
- Turn settle 后 Host 将 assistant 正文以 `markdown_text` 发出；CLI **始终**用 goldmark→ANSI 渲染（流式回复也能看到 Markdown）。
- 无 settle 信号时，`end()` 用 TurnResult.Assistant 兜底渲染。

### 5. Command 平面

**原生命令（v1，仅此五个）：**

| 命令 | 作用 |
|---|---|
| `/help [plugin]` | 全局帮助，或某插件的命令表 |
| `/lp` | 列出已挂载插件（name/version/provides/description） |
| `/refresh` | 重扫插件目录，更新内存 Manifest（不热插拔） |
| `/exit` | 唯一退出方式（移除裸 `exit`/`quit`） |
| `/` + 回车 | 等价 `/help` |

- 未知命令：`unknown command: foo` + 编辑距离 ≤2 的「did you mean」。
- Tab 补全：`golang.org/x/term` raw 模式；补全原生命令、插件名、插件子命令；歧义时列出候选。

**插件命令：**

- Manifest 新增 `description` 与 `commands[]`：

```json
{
  "name": "llm-openai",
  "description": "OpenAI-compatible LLM provider",
  "commands": [
    {
      "name": "config",
      "description": "Show or set API key / model / baseURL",
      "usage": "/llm-openai config [get|set key=value]"
    }
  ]
}
```

- 调用：`/插件名 子命令 参数…`
- 路由 Frame：`cap=commands, method=call, payload={"command":"config","args":"set k=v"}` 发往目标插件进程。
- 非空 `commands[]` 隐含提供该面；`provides` 不必重复声明。
- 插件未实现 `commands.call` handler：mount 探测 warn，不拒载。
- `/help 插件名` 直接从 Manifest 渲染，不 Call 插件。
- 无 commands 的插件在 `/help` 里只显示名称+description。

**冲突策略：**

- 插件 name 小写 ∈ 原生命令名集合 → **整个插件不挂载**，启动 warn。
- name 规范：`[a-z0-9-]+`，Discovery 校验。

**配置写入（llm-openai 示例）：**

- `/llm-openai config set apiKey=…` 写插件目录 `config.json`（gitignore 已覆盖）。
- 优先级保持 **env > config.json**；命令只是改 config.json 的交互入口。
- 不引入全局配置文件。

### 6. 对齐 Claude Code 的本轮边界

本轮做：taxonomy + goldmark + 工具 summary 卡 + thinking dim + slash command 骨架 + `/` 预览（列表/did-you-mean）。

后置 backlog（不在本 spec）：

- 流式 markdown 增量渲染
- 权限确认（y/n）
- `/clear` `/compact`
- 彩色 diff / patch 渲染
- Spinner 进度
- Tab 补全 / readline
- 热插拔（/refresh 级）
- 独立 Web/Desktop Render Medium

## User Stories

1. 作为使用者，模型正文以完整 Markdown（表格/列表/代码块）呈现在终端。
2. 作为使用者，工具调用开始看到一行 `Running …`，结束后看到键值摘要卡，过长参数被截断。
3. 作为插件作者，我调用 `EmitMarkdownText` / `EmitMessageText` / `EmitSummaryText` 向主窗口发渲染意图。
4. 作为使用者，输入 `/help` 看到原生命令与插件命令；`/lp` 看到已挂载插件。
5. 作为使用者，`/llm-openai config set apiKey=sk-xxx` 在主窗口完成配置，无需改文件或 env。
6. 作为部署者，装了与 `/lp` 同名的插件时，启动即看到拒绝提示，而不是运行期神秘失败。
7. 作为使用者，仅 `/exit` 退出；输入 `/` 能看到命令预览。

## Implementation Decisions

- **三分类替换而非扩展**：expandable 退役，避免四分类认知负担。
- **goldmark 而非 glamour**：只引解析器，ANSI 风格仍归本仓库，依赖面最小。
- **工具渲染 Host 统一发**：无 Presentation 的工具也有默认卡；插件 Card 仍可叠加。
- **summary_text 截断只影响显示**：Session Log 事实完整。
- **protocol bump v2 + SDK 直接换名**：仓库内无外部插件用户，不留兼容 shim。
- **命令冲突拒载整个插件**：保留名冲突是配置错误，静默砍命令面更难排查。
- **Manifest 刷新只更元数据**：热插拔独立议题。
- **commands 不进 provides**：manifest 一处真源。
- **config.json 为唯一可写配置面**：不引入全局配置文件。

## Testing Decisions

- 主缝不变：真实 Host + fixture 插件进程。
- taxonomy：发旧 kind 的帧被丢弃且有 warn；新 kind 往返一致。
- summary_text：Host 工具路径必发；截断 rune 边界（中文）有测。
- markdown：goldmark 输出含表格/列表的 ANSI；`mdansi.Plain` 宽度测保留。
- commands：`/help` `/lp` `/exit` `/refresh` 冒烟；`/plugin subcmd` 路由到插件 handler；冲突插件不挂载且 stderr 有提示；`/help plugin` 只读 Manifest。
- settle：流式仅显示进度行；markdown_text 始终全量渲染。
- Tab：`completeSlash` 对 `/h`、`/`、`/plugin ` 有单测。
- Card / Session Log 不变量不回归。

## Out of Scope

- 流式**增量** markdown 高亮（settle 全量已做）、权限确认（sandbox）、diff 高亮。
- Session 持久化 / `/clear` 清 Session / `/compact`。
- 热插拔、多 Render Medium 实例、Web UI。
- 真实多 provider 目录、token 计量。

## ADR（确认后落盘）

- **ADR-0007** presentation-text-taxonomy：三分类替换、expandable 退役、protocol v2、SDK 去旧名。
- **ADR-0008** cli-command-plane：Manifest commands 声明 + Host 路由；冲突即拒载。

## Issue 分解（建议）

| # | 标题 | 依赖 |
|---|---|---|
| 01 | protocol v2 + RenderKind 更名 + SDK 三方法 | — |
| 02 | goldmark 接入 mdansi（AST→ANSI） | — |
| 03 | Host 工具路径 message_text/summary_text + settle markdown_text | 01 |
| 04 | turnRenderer 消费新 taxonomy + thinking dim | 01,02,03 |
| 05 | Manifest 扩展（description/commands）+ Discovery 校验/冲突拒载 | — |
| 06 | REPL Command 平面（/help /lp /refresh /exit / 路由） | 05 |
| 07 | llm-openai `config` 命令写 config.json | 05,06 |
| 08 | README / examples 同步 | 01–07 |

## Further Notes

- CONTEXT.md 已同步：Presentation 三分类、新增 Command / Manifest。
- 术语以 CONTEXT.md 为准；约束以 ADR-0001–0008 为准（0007/0008 待落）。
- `fakellm` / 测试 fixture 保留。
