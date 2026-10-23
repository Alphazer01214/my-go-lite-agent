# 插件开发标准

> 状态：当前实现
> 说明：本文规定 LiteAgent 插件的目录组织、文件划分与代码结构。契约层见 [00-conventions.md](../design/plugins/00-conventions.md)，开发流程与打包见 [plugin-dev.md](plugin-dev.md)。

本文只管"代码放在哪、叫什么、怎么拆"。插件的业务语义、错误码含义、UI 契约不在这里。

## 1. 一条判定命题

> **平台概念只能有一个事实来源。**
> 如果两段代码在两个插件里长得一样，它就不属于插件。

这条命题直接推出三件事。

| 归属 | 内容 | 落点 |
|---|---|---|
| 平台 | 帧传输、错误辅助、WebUI 桥接 | `protocol/`、`pluginsdk/` |
| 插件 | 能力语义、业务状态、投影逻辑、策略 | `plugins/<name>/` |
| 插件私有 | 适配类型、临时结构 | 插件自己的目录内 |

**唯一的例外是工具提供方契约。** 它是 `tools` 插件与 Provider 插件之间的业务契约，归 `tools` 插件所有，但 Provider 侧各留一份本地副本（`wire.go`）。

代价是协议改动要改多份。换来的是平台树不含任何插件私有格式，Provider 之间不互相依赖。

反过来这条也成立：**平台树里不许出现插件信息。** `protocol/`、`internal/`、`pluginsdk/`、`cmd/` 都不知道具体有哪些插件、有哪些能力、工具信封长什么样。

## 2. 目录槽位

每个插件目录由固定槽位加若干功能文件组成。

```text
plugins/<name>/
  plugin.json          # 静态注册面。必填。
  README.md            # 人读。必填。
  communication.md     # 方法、payload、Event 与前端通信。必填。
  main.go              # 进程入口：注册 handler，装配依赖。
  errors.go            # 本插件的错误码常量与构造器。
  types.go             # 跨功能共享的契约类型。
  plugin.go            # plugin 结构体、配置、锁与生命周期。
  <feature>.go         # 功能文件，按关注点切。
  <feature>_test.go
  ui/                  # 前端资源，按需。
```

| 槽位 | 放什么 | 不放什么 |
|---|---|---|
| `main.go` | handler 注册、依赖装配、`Serve()` | 业务逻辑 |
| `errors.go` | 本插件错误码常量、具名构造器 | 平台错误码（用 `pluginsdk` 的） |
| `types.go` | 被两个以上功能域共享的契约类型 | 某个功能域专属的类型 |
| `plugin.go` | plugin 结构体、config、锁、生命周期状态 | 业务规则 |
| `<feature>.go` | 一个关注点的全部：类型、专属 payload、逻辑、编码 | 别的关注点 |

`config.go` 是常见的功能文件名，专门放配置的读取、校验和持久化。

`config.json` 是运行期可写配置，不提交 Git。`config.example.json` 是配置模板，随发行版分发。

### 2.1 `wire.go` 是 Provider 侧本地副本

工具提供方的调用契约是 `tools` 插件与 Provider 插件之间的业务契约，不是 Host 契约。

所以它不进 `pluginsdk`。`pluginsdk` 只共享 JSON 字段名，不共享 Go 类型。Provider 侧各维护一份本地副本，文件名统一为 `wire.go`。

多份副本是**有意的代价**，不是待清理的重复。契约归 `tools` 插件所有，平台树只认 JSON 字段名；Provider 之间不互相依赖。

| 归属 | 内容 |
|---|---|
| `pluginsdk/` | 帧传输、错误辅助、WebUI 桥接。**不含任何插件私有格式** |
| `protocol/` | Host 线契约 |
| `plugins/tools/wire.go` | tools 插件的注册与调用信封（工具插件自己的副本） |
| `plugins/<provider>/wire.go` | Provider 侧本地副本 |

当前持有 `wire.go` 的插件是 `tools`、`file-tools`、`shell-tools`、`sandbox` 和 `subagent`，共五份。

Provider 侧副本要抄全：`Definition`、`ProviderInput`、`Result`、`RegisterInput`、`SyncInput`、`SyncOutput`、`Caller`，以及编解码函数。

改协议时三份一起改。`plugins/tools/communication.md` 是这份契约的文档真源。

### 2.2 为什么没有 `util.go`

`util.go` 装不下任何确定的东西。它会长成杂物袋，而且没人能说出里面该放什么。

本项目把这类函数按归属拆：

| 函数 | 去处 |
|---|---|
| `hashBytes` | model-context 的 `prompt.go`；file-tools 的 `util.go` |
| `hashString`、`hashJSON` | model-context 的 `prompt.go` |
| `truncateUTF8` | file-tools 的 `util.go`；model-context 的 `compact.go` |
| `truncateString` | `tools` 与 `shell-tools` 的 `wire.go`（截断信封正文用） |
| `displayPath`、`joinRelative` | file-tools 的 `util.go` |
| `sortedUnique`、`formatInt`、`intDefault` | file-tools 的 `util.go`（仅本插件使用） |

一个插件内部用得到的通用小函数可以留在自己的 `util.go`。但它不能出现在两个插件里，除非它是工具提供方契约的一部分——那属于 `wire.go`。

## 3. 文件命名

**文件名必须能回答"这个文件里是什么"。**

| 类别 | 例子 | 说明 |
|---|---|---|
| 领域名 | `read.go`、`policy.go`、`projection.go`、`budget.go` | 首选 |
| 平台槽位 | `main.go`、`errors.go`、`types.go`、`plugin.go` | 固定名，职责见第 2 节 |
| 契约槽位 | `wire.go` | 仅 Provider 插件。工具提供方契约的本地副本，见第 2.1 节 |
| 禁止 | `builder.go`、`handler.go`、`manager.go`、`helper.go`、`common.go`、`misc.go` | 技术角色名。按它拆完仍然是一坨 |

`builder.go`、`handler.go` 这类名字的问题是它们描述的是**代码的形态**而不是**代码的职责**。`turn.go` 描述职责，`builder.go` 不描述。

按操作系统拆文件是例外。`process_unix.go` 与 `process_windows.go` 用构建标签分开，文件名加后缀即可。

## 4. 拆分轴

> **拆分的轴只能是关注点，不能是技术类型。**

反例是按技术类型分：`types.go` 装所有类型、`util.go` 装所有工具函数、`store.go` 装所有存储。这种分法拆完每个文件还是一坨，因为一个文件里仍然混着多个关注点。

正例是按关注点分：

| 插件类型 | 分法 | 例子 |
|---|---|---|
| 能力型（提供工具） | 一个工具一个文件 | file-tools：`read.go` / `write.go` / `list.go` / `glob.go` / `search.go` / `stat.go` |
| 流程型（编排） | 一个阶段一个文件 | model-context：`assemble.go` / `budget.go` / `compact.go` / `snapshot.go` / `methods.go` |
| 展示型 | 一个界面一个文件 | session：`ui_chat.go` / `ui_trace.go` / `ui_dock.go` / `ui_select.go` |

### 4.1 触发拆分的信号

| 信号 | 处理 |
|---|---|
| 一个文件里有三组以上不同动词前缀的顶层函数 | 按动词族拆 |
| 一个文件同时做 IO 与纯计算 | 纯计算部分拆出去 |
| 一个文件承载两个以上界面或阶段 | 按界面或阶段拆 |
| 一个函数里出现三种以上数据结构 | 按数据结构抽子步骤 |

不设行数上限。文件该多长由职责决定，不由数字决定。判断标准是"这个文件能不能用一句话说清它做什么"。

## 5. 共享度规则

`types.go` 装什么，不由类型名决定，由**被几个功能域使用**决定。

对每个类型问三句：

1. 它是某个 `capability.method` 的 payload 吗？不是 → 归它所属的功能文件。
2. 它被两个以上功能域使用吗？不是 → 归它所属的功能文件，**payload 也不例外**。
3. 是 → 进 `types.go`。

这条规则让同一类插件可以有两种正确形态。

| 情况 | 形态 |
|---|---|
| payload 只服务于一个方法 | 跟功能文件走。`file-tools` 的 `readIn` 留在 `read.go` |
| payload 被多个方法共用 | 进 `types.go`。session 的 `factIn` 被 store、session_ops、lifecycle 共用，进 `types_ops.go` |

在 `types.go` 里，In 与 Out 放在一起。调用方读一个契约面只需要打开一个文件。

payload 类型按拆分轴细分时，可以另开 `types_<concern>.go`。session 的 `types.go`、`types_ops.go`、`types_workspace.go` 是这种形态。

## 6. 错误码

**错误码字面量只能出现在 `errors.go`。** 其他文件一律用常量或构造器。

```go
// errors.go
package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
    codePathEscape   = "path_escape"
    codePathNotFound = "path_not_found"
)

// Constructors keep the code and the message together at the raise site.
func errPathEscape(msg string) error { return pluginsdk.ErrCode(codePathEscape, msg) }
```

调用点写 `errPathEscape("path is outside the workspace")`，不写 `pluginsdk.ErrCode("path_escape", ...)`。

工具结果包络里的错误码用常量，因为包络函数自己收 code 参数：

```go
return wire.Failure(codeFileBinary, "file contains NUL bytes", meta)
```

### 6.1 平台错误码不重复定义

`bad_arguments`、`handler_error`、`tool_result_invalid` 归平台。插件直接用 `pluginsdk` 的构造器，不在自己的 `errors.go` 里再抄一份。

### 6.2 一个例外：session

session 的 `errors.go` 用本地 `coded` 类型，不走 `pluginsdk.ErrCode`。

原因是行为差异：session 的错误走本地类型时，`pluginsdk.Code(err)` 返回空串；换成 `pluginsdk.ErrCode` 会让线上 `error_code` 从空变成实际错误码。session 保留本地类型是有意的，不是遗留。

## 7. 依赖方向

```text
errors.go   types.go        ← 叶子，只依赖 SDK 与标准库
    ↓          ↓
          plugin.go            ← 生命周期
    ↓          ↓
       <feature>.go            ← 功能实现
    ↓
          main.go               ← 只做装配
```

插件之间不直连。跨插件调用一律发 Call，由 Host 路由。路由键是 `(capability, method)`，插件名不参与寻址。

`wire.go` 是例外路径。它复制的是 JSON 字段名和编解码逻辑，不 import 任何插件包。

## 8. 迁移记录

本轮重构只搬不改。每个插件的顶层声明集合在重构前后完全一致，错误码的集合与调用次数也完全一致。

| 插件 | 变化 |
|---|---|
| session | `ui_state.go` 1651 行拆成 8 个文件；`store.go` 617 行拆成 4 个；`types.go` 拆成 3 个 |
| model-context | `builder.go` 1153 行拆成 9 个；`types.go` 326 行拆成 3 个 |
| agent | `turn.go` 910 行拆成 5 个 |
| llm-openai | `config.go` 520 行拆成 5 个 |
| file-tools、shell-tools、tools | 入口文件拆出工具定义表；`wire.go` 保持各插件本地一份 |
| sandbox | `main.go`、`methods.go` 按 seam 与方法族拆开 |
| 全部 | 错误码字面量归入各自的 `errors.go` |

工具提供方契约没有归并到 `pluginsdk`。五份 `wire.go` 保持本地副本，平台树不含插件私有格式。

## 9. 验收

| 项 | 检查方式 |
|---|---|
| 目录名一致 | 目录名 == `plugin.json.name` == `pluginsdk.NewPlugin` 参数 |
| 必备文件齐备 | `main.go`、`errors.go`、`plugin.json`、`README.md`、`communication.md` 存在 |
| 平台树无插件信息 | `protocol/`、`internal/`、`pluginsdk/`、`cmd/` 不出现插件名、插件能力名或插件私有结构体 |
| 契约归属正确 | 工具提供方契约在 Provider 插件的 `wire.go`，不在 `pluginsdk/` |
| 错误码归位 | 全仓检索错误码字面量，只在 `errors.go` 出现 |
| 依赖方向 | 插件不 import 其他插件的包 |
| 声明集合不变 | 拆分前后顶层声明集合逐项相等 |

必备文件一项当前有偏差：`subagent` 还没有 `errors.go` 和 `communication.md`。补齐之前它不满足本节的验收标准。

## 10. 与其他文档的关系

| 文档 | 关系 |
|---|---|
| [00-conventions.md](../design/plugins/00-conventions.md) | 契约层：命名、能力、错误码、Manifest |
| [plugin-dev.md](plugin-dev.md) | 开发流程与打包 |
| [pluginsdk.md](pluginsdk.md) | 插件开发接口 |
| [prompts/](../prompts/README.md) | 模型侧文本的规格。插件按它贡献内容 |
| [protocol.md](protocol.md) | 帧格式与路由 |
