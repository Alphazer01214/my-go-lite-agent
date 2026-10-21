# Prompt 写作原则

> 状态：目标契约

本文给出 30 条原则。每条包含三部分：原则本身、出处、本项目落点。

出处格式为 `项目/路径:行号`。行号指向被验证过的源码位置。

## 1. 结构

**1. 一节只做一件事。** 身份、系统语义、任务观、风险观、工具观、语气、效率各占一节，节内是规则清单。

- 出处：claude-code-main `src/constants/prompts.ts:560-576`；codex `codex-rs/models-manager/prompt.md:11-333`；pi `coding-agent/src/core/system-prompt.ts:121-180`
- 落点：分节词表见 [02-system-prompt.md](02-system-prompt.md) 第 2 节。

**2. 小节加短条目。** 一条规则一行，一两句话读完。模型对短指令的遵循度更高，prompt 也因此可以按行 diff、按行去重。

- 出处：deepseek-harness `packages/core/system-prompt/src/index.ts:447-461`；pi `core/tools/edit.ts:48`
- 落点：单节正文不超过 12 条，条目不超过 2 句。

**3. 身份句极简，但砍不得。** 第一句是自我认知锚点，哪怕精简到极致也要保留。

- 出处：claude-code-main `src/constants/prompts.ts:450-454`；deepseek-harness `packages/core/system-prompt/src/index.ts:429`
- 落点：`identity` 节固定一句，位置最前，不允许被配置覆盖为空。

**4. 身份与能力分离。** 身份稳定，能力随工具集变。换工具集时，身份段不动，前缀仍然可缓存。

- 出处：pi `coding-agent/src/core/system-prompt.ts:148-151`；deepseek-harness-x `packages/core/system-prompt/src/index.ts:56-60`
- 落点：身份在静态轨，工具纪律在静态轨的条件节内，见 [07-assembly-cache.md](07-assembly-cache.md)。

**5. 可替换人格做成独立节。** 人格是数据，不是散落全文的措辞。人格缺席时整段切除。

- 出处：codex `codex-rs/models-manager/src/model_info.rs:50-96`
- 落点：`persona` 节独立可配，`order` 固定在 `identity` 之后。

**6. 分节带排序键。** 节有稳定的顺序键，渲染时按排序键拼接，不依赖配置书写顺序。

- 出处：deepseek-harness-x `packages/core/system-prompt/src/index.ts:56-60`；deepseek-harness 的 `SECTION_ORDERS` 位次表
- 落点：`prompt_sections[].order` 承担排序，见 [02-system-prompt.md](02-system-prompt.md) 第 3 节。

**7. 节分两轨。** 静态轨字节稳定，动态轨每轮可变。两者之间是缓存边界。

- 出处：claude-code-main `src/constants/prompts.ts:105-113`；ZCode `builder.ts:230-277`
- 落点：`prompt_sections[].track`，见 [07-assembly-cache.md](07-assembly-cache.md) 第 2 节。

## 2. 语气与规则表述

**8. 第二人称直呼模型，谈用户用第三人称。** 把模型放在旁观执行者的位置，减少它代入用户视角替你做决定。

- 出处：claude-code-main `src/constants/prompts.ts:222`
- 落点：模板中用 `the user`，不用 `you` 指代用户。

**9. 默认人格一句话锚定。** 不要指望模型自己猜语气。

- 出处：codex `codex-rs/models-manager/prompt.md:15`
- 落点：`communication` 节第一句固定语气锚点。

**10. 语气可以分级。** 不同场景允许不同语气，但同一场景内不要摇摆。

- 出处：codex `codex-rs/models-manager/prompt.md:121`；pragmatic 人格 `core/templates/personalities/gpt-5.2-codex_pragmatic.md:15`
- 落点：v1 只有一套语气，模式差异留在运行时上下文，不进静态轨。

**11. 祈使句加条件限定，保留合法例外。** `Do not create files unless ...`，不要写成 `Never create files`。

- 出处：claude-code-main `src/constants/prompts.ts:231`；ZCode `apps/zcode-cli/packages/core/src/tool/handlers/bash-prompt.ts:14`
- 落点：禁令条目必须带 `unless`、`only when` 或 `after you verified` 之类的限定。

**12. 每条规则附代价账。** 不说"要谨慎"，说清误操作的代价。

- 出处：claude-code-main `src/constants/prompts.ts:258`
- 落点：`task_policy` 节的每条高危规则都写清代价。

**13. 规则引用真实策略名。** 引用策略名而不是复述逻辑，模型才能泛化到没列举的场景。

- 出处：deepseek-harness `packages/fs/tool-fs/src/edit.ts:81`
- 落点：涉及工具的规则引用工具名或能力名，不引用内部实现细节。

**14. 正反对照，防"过度"与"不足"两个方向。** `完成到底——不要镀金，也不要半途而废。`

- 出处：claude-code-main `src/constants/prompts.ts:758`；MiMo-Code `packages/opencode/src/session/prompt/default.txt:23`
- 落点：`task_policy` 节同时给出上限和下限。

**15. 用数字代替形容词。** 数字没有解释空间。

- 出处：codex `codex-rs/models-manager/prompt.md:111,230,266,276,285`；MiMo-Code `agent/prompt/title.txt:10`
- 落点：输出格式节固定 4 组量化约束，见 [02-system-prompt.md](02-system-prompt.md) 第 6 节。

**16. 否定偏好要堵默认习惯。** `用读取工具——不要用 shell 的 cat` 比 `请用读取工具` 更有效。

- 出处：deepseek-harness-x `packages/fs/tool-fs/src/read.ts:73`
- 落点：工具纪律节明确给出替代动作，见 [03-tool-prompts.md](03-tool-prompts.md) 第 3 节。

**17. 可判定门禁。** 把诚信与质量要求变成可验证条件。

- 出处：grok-build `templates/prompt.md:18`；ZCode `apps/zcode-cli/packages/core/src/context/sections/workflow-actor.ts:31`
- 落点：`task_policy` 节含一条"只声明有输出支撑的完成"。

**18. 优先级链要显式。** 让模型在没有人类仲裁时自行消解冲突。

- 出处：grok-build `templates/apply_patch_prompt.md:513-522`；ZCode `context/sections/request-user-context.ts:67`
- 落点：AGENTS.md 分层规则见 [04-runtime-injection.md](04-runtime-injection.md) 第 4 节。

**19. 事实、禁止、替代三段式。** 先陈述运行时事实，再据此禁止，最后给替代动作。只禁不导会让模型无所适从。

- 出处：deepseek-harness-x `packages/jobs/tool-jobs/src/index.ts:266`
- 落点：所有"禁止轮询""禁止直连"类条目必须给出替代能力名。

**20. 反模式清单点名具体坏例。** 不讲空泛的"要简洁"，而是点名 `片段、缩写、箭头链` 这类具体坏形态。

- 出处：ZCode `context/dynamic-sections.ts:17`
- 落点：`communication` 节列 5 到 8 个具体坏例。

## 3. 示例与强调

**21. 成对示例加推理依据。** "什么时候用、什么时候不用"这种判断，具象化才讲得清。

- 出处：claude-code-main `src/tools/TodoWriteTool/prompt.ts:29-121`
- 落点：v1 只在工具描述里给 Good/Bad 对照，见 [03-tool-prompts.md](03-tool-prompts.md) 第 4 节。

**22. 好坏对照锚定质量下限。** 给高质量和低质量各若干例，再收一句"只写高质量的"。

- 出处：codex `codex-rs/models-manager/prompt.md:149-196`
- 落点：同上。

**23. 给完整可复制的模板。** 结构化工具配文法定义，调用错误率明显下降。

- 出处：claude-code-main `src/tools/BashTool/prompt.ts:120-125`；grok-build `crates/codegen/xai-grok-tools/src/implementations/codex/apply_patch/tool.rs:26-96`
- 落点：`file.patch` 的 `edits` 参数描述给出最小示例。

**24. 示例按当前 schema 条件渲染。** 不教模型调用不存在的签名。

- 出处：deepseek-harness `packages/core/tools/src/ts-types.ts:270-282`
- 落点：示例从 `input_schema` 派生，不手写第二份。

**25. 大写情态动词限量。** 全项目只固定用 NOT、ONLY、EXACT 少数几个，且只出现在防跑偏与格式锁定两类位置。

- 出处：pi `coding-agent/src/core/compaction/compaction.ts:562-568`
- 落点：静态轨全量只允许 3 个大写词：`NOT`、`ONLY`、`EXACT`。

**26. 首尾重复加失败后果。** 禁令写在最前和最后，并附后果。

- 出处：claude-code-main `src/services/compact/prompt.ts:12-26`；ZCode `compact/prompt.ts:1-12`
- 落点：压缩 prompt 骨架见 [05-compaction.md](05-compaction.md) 第 2 节。

**27. 跨位置重复补漏。** 同一条规则在主 prompt 和子代理画像各写一遍。子代理不一定看得到主 prompt。

- 出处：claude-code-main `src/constants/prompts.ts:438` 与 `:770`；rea `src/contracts/promptContracts.ts:84`
- 落点：子代理画像独立成文，见 [06-subagent.md](06-subagent.md)。

**28. 标签分区。** 自定义标签划定内容域，让模型区分框架事实、用户指令和待渲染内容。

- 出处：codex `collaboration-mode-templates/templates/plan.md:102`
- 落点：保留标签清单见 [04-runtime-injection.md](04-runtime-injection.md) 第 2 节。

**29. 干预分级再终止。** 给模型自我修正机会，同时保证最终收敛。

- 出处：MiMo-Code `session/prompt/text-loop-recovery.ts:20-40`；deepseek-harness-x `packages/guard/repeat-tool-reminder/src/index.ts:63-79`
- 落点：v1 只做两级，见 [04-runtime-injection.md](04-runtime-injection.md) 第 6 节。

## 4. 工具描述

**30. 工具描述是 prompt 体系的另一半。** 统一配方：首句能力、行为语义、边界、防绕过、条件注入。再配"什么时候用、什么时候不用"。

- 出处：claude-code-main `src/tools/BashTool/prompt.ts:354-368`；deepseek-harness `packages/shell/tool-bash/src/index.ts:90-96`
- 落点：完整配方见 [03-tool-prompts.md](03-tool-prompts.md) 第 2 节。

配套三条军规：

| 军规 | 说明 | 出处 |
|---|---|---|
| 工具名不写死 | 常量插值或模板占位，重命名不撕裂 prompt | claude-code-main `src/constants/prompts.ts`；grok-build `types/template_renderer.rs:48-49` |
| schema 讲单次，prompt 讲跨调用 | 一次调用怎么用写 schema，跨调用纪律写 prompt 段 | deepseek-harness `packages/shell/tool-bash/src/index.ts:235` |
| 参数描述承载不变量 | 默认值、单位、上限、唯一性约束前移成模型侧约束 | pi `core/tools/edit.ts:23-26` |

## 5. 本项目落点汇总

| 原则群 | 本项目载体 | 文档 |
|---|---|---|
| 结构 1-7 | 静态轨与动态轨的分节词表 | [02-system-prompt.md](02-system-prompt.md) |
| 语气 8-20 | 主 system prompt 全文模板 | [02-system-prompt.md](02-system-prompt.md) |
| 示例 21-29 | 压缩、委派、注入三类一次性 prompt | [05-compaction.md](05-compaction.md)、[06-subagent.md](06-subagent.md) |
| 工具 30 | 工具描述配方与三件套 | [03-tool-prompts.md](03-tool-prompts.md) |
| 防退化 | 不变量与测试 | [08-invariants.md](08-invariants.md) |

## 6. 反模式

下列做法在基准工程中被明确避免。本项目同样不采用。

| 反模式 | 为什么不行 |
|---|---|
| 一篇文章式的巨型 prompt | 无人采用；长 prompt 的遵循度随位置衰减 |
| 工具使用说明镜像进 system prompt | 与工具字段重复，制造两份事实 |
| prompt 描述不存在的工具或参数 | 制造幻觉能力，模型会尝试调用 |
| 把外部内容当指令 | 外部内容可被构造成指令 |
| 只给禁令不给替代动作 | 模型无所适从，转而绕过 |
| 裸插值不报错 | 拼写错误静默生效，很难排查 |
| 用户消息被当作系统通知的批准 | 越权路径 |
| 无限追加规则 | 强调通货膨胀等于没有强调 |
