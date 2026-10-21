# Prompt 设计

> 状态：目标契约
> 说明：本目录定义 Prompt 契约。已落地的部分在 [plugins/model-context](../../plugins/model-context) 的实现里，阶段划分见 [09-plan.md](09-plan.md)。

本目录定义 LiteAgent 的 Prompt 契约。

它回答四个问题：prompt 怎么写、怎么分节、怎么拼装、怎么防止退化。

基准来自一批开源 agent 工程。它们在写作方式上高度收敛，本目录提炼其中的共识，并适配 LiteAgent 的运行时约束。

## 1. 范围

| 属于本目录 | 不属于本目录 |
|---|---|
| 主 system prompt 的分节与排序规则 | 某个插件的业务语义 |
| 工具描述的写法与三件套分工 | 工具的实现 |
| 运行时注入的信封与转义规则 | 注入内容的业务来源 |
| 压缩与摘要的 prompt 资产 | 压缩策略与预算计算 |
| 子代理画像与委派元指令 | 子代理的调度与并发 |
| 缓存边界与 token 预算口径 | 具体模型的窗口数值 |
| Prompt 不变量与测试要求 | 插件的业务测试 |

插件的业务契约写在 [docs/design/plugins/README.md](../design/plugins/README.md)。平台协议写在 [protocol.md](../platform/protocol.md) 与 [manifest.md](../platform/manifest.md)。术语见 [CONTEXT.md](../../CONTEXT.md)。

## 2. 文档索引

| 文档 | 内容 |
|---|---|
| [01-principles.md](01-principles.md) | 30 条写作原则，每条给出出处、理由和本项目落点 |
| [02-system-prompt.md](02-system-prompt.md) | 主 system prompt 的分节词表、排序、变量和英文全文模板 |
| [03-tool-prompts.md](03-tool-prompts.md) | 工具描述配方、三件套分工、参数不变量写法 |
| [04-runtime-injection.md](04-runtime-injection.md) | 注入信封、标签转义、AGENTS.md 分层、注入点清单 |
| [05-compaction.md](05-compaction.md) | 压缩 prompt 骨架、恢复消息、迭代压缩合并规则 |
| [06-subagent.md](06-subagent.md) | 子代理画像三段式、父代理的委派元指令 |
| [07-assembly-cache.md](07-assembly-cache.md) | 拼装架构、缓存边界、动态注入点与降级顺序 |
| [08-invariants.md](08-invariants.md) | Prompt 不变量、测试写法与防退化清单 |
| [09-plan.md](09-plan.md) | 落地阶段、验收标准和取舍 |
| [10-task-prompts.md](10-task-prompts.md) | 15 条可直接粘贴的任务级 prompt 全文、摆放位置与验收标准 |
| [11-vendor-picks.md](11-vendor-picks.md) | 九家各挑一条最优，含原文、本项目落点与明确不抄的 |
| [12-assets.md](12-assets.md) | **可直接使用的 prompt 文本**：静态轨八节、可粘贴的 config.json、9 个工具的描述、压缩与子代理 prompt |
| [13-application.md](13-application.md) | **session/model-context 如何应用这些 prompt**：职责切分、时序、落点映射、三条落地路径 |

### 2.1 快速上手

想直接看到效果，看 [12-assets.md](12-assets.md)。第 3 节是一份可以原样写进 `plugins/model-context/config.json` 的配置，不改任何 Go 代码就能生效。

想知道为什么抄这些，看 [11-vendor-picks.md](11-vendor-picks.md)。九家各一条，每条都带原文和本项目落点。

想知道这些文本怎么进到模型手里，看 [13-application.md](13-application.md)。第 4 节是资产到落点的完整映射。

### 2.2 两层资产

| 层 | 文档 | 特征 | 放哪 |
|---|---|---|---|
| 长期身份 | [12-assets.md](12-assets.md) | 主 system prompt、工具描述、压缩、子代理画像 | 静态轨或工具表，跟着会话一直生效 |
| 一次性任务 | [10-task-prompts.md](10-task-prompts.md) | P01 到 P15 | user 消息首段，用完即止 |

两层的分工见 [12-assets.md](12-assets.md) 第 1.1 节。

## 3. 三条基本判断

### 3.1 Prompt 是代码

改 prompt 等于改一个受版本控制、有测试覆盖的函数。

Prompt 文本不写在业务代码里，也不散落在多处。它集中在少数模板函数中，运行期值一律插值。

### 3.2 分节优于长文

九个基准工程没有一个采用"一篇文章式的巨型 prompt"。

共同形态是：静态分节前缀 + 动态注入层 + 工具描述协同 + 一次性任务 prompt。

### 3.3 模型侧 prompt 用英文

九个基准工程的模型侧 prompt 全部用英文。

这是默认选择，不是风格偏好。训练数据与指令遵循的偏置都在英文侧。

平台文档用中文，模型可见文本用英文。两者边界清晰，不要混写。

## 4. 状态标注

本文档区分三种状态：

| 标注 | 含义 |
|---|---|
| 当前实现 | 代码里已经生效的行为 |
| 目标契约 | 已经定下、待实现的行为 |
| 暂不实现 | 明确排除，本轮不写代码 |

只写清楚这三类，读者才知道哪些能照抄，哪些要等。

## 5. 引用来源

原则与资产的原始出处写在 [01-principles.md](01-principles.md) 与 [02-system-prompt.md](02-system-prompt.md) 的表格里。

来源项目为：`claude-code-main`、`codex`、`deepseek-harness`、`deepseek-harness-x`、`grok-build`、`MiMo-Code`、`pi`、`rea`、`ZCode`。

本目录是对这些工程公开行为的综合再创作。短句与出处相同，是为了保留被验证过的措辞。
