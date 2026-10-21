# 工具描述

> 状态：目标契约

工具描述是模型选择工具的唯一依据。它承担 prompt 体系的另一半。

## 1. 三件套

基准工程把工具的模型侧文本拆成三块，互不重叠。

| 块 | 内容 | 落在哪里 |
|---|---|---|
| `description` | 这个工具做什么，返回什么 | 工具字段 |
| `prompt_snippet` | 一句话进工具清单 | 工具字段，可选 |
| `guidelines` | 这一类调用的跨调用纪律 | 工具字段，进静态轨 `tool_discipline` 节 |

分开的理由是"一个事实只出现一次"。

`description` 面向选工具，只讲这一类工具的共同语义。`guidelines` 面向行为，讲多次调用之间的纪律。

schema 面向单次调用，讲参数怎么填。schema 与 `guidelines` 重复的部分要删掉。

- 出处：pi `core/tools/{bash,edit}.ts:43-51`；deepseek-harness `packages/shell/tool-bash/src/index.ts:235`

## 2. 描述配方

工具描述按固定顺序写，五段。

```text
<首句：一句话能力陈述，写清做什么、返回什么。>

<行为语义：工作目录是否持久、会话是否持久、超时与截断的默认值和上限、
  超限输出的落盘位置。>

<边界：截断策略、落盘行为、失败返回格式。>

<防绕过：说明为什么用这个工具而不是绕路。>

<When to use / When NOT to use：什么时候用，什么时候不要用。>
```

- 出处：claude-code-main `src/tools/BashTool/prompt.ts:354-368`；codex `codex-rs/core/src/tools/handlers/shell_spec.rs:97-141`；deepseek-harness `packages/shell/tool-bash/src/index.ts:90-96`；ZCode `apps/zcode-cli/packages/core/src/tool/handlers/bash-prompt.ts:1-23`

### 2.1 运行时值插值

描述里的数值从配置插值，不写死。

超时上限、默认输出上限、禁用的命令列表都随配置变化。写死后配置就失去了意义。

- 出处：claude-code-main `src/tools/BashTool/prompt.ts:335`；ZCode `apps/zcode-cli/packages/core/src/tool/handlers/bash-prompt.ts:6-8`

### 2.2 参数描述承载不变量

运行时校验规则要前移成模型侧约束。

```text
old_text   必填。待替换的原文，必须在文件中唯一出现，且不与另一处编辑重叠。
timeout_ms 可选。默认 120000，上限 600000。超过上限按超时处理。
```

唯一性、上下限、默认值、单位、互斥关系都写进参数描述。

- 出处：pi `core/tools/edit.ts:23-26`；codex `codex-rs/core/src/tools/handlers/shell_spec.rs:133-135`

### 2.3 失败返回格式

工具失败分两类，返回形态不同。

| 类型 | 形态 | 模型读到的 |
|---|---|---|
| 参数或权限错误 | 协议错误码 | 一句可读的原因 |
| 业务失败 | `is_error: true` 加正文 | 失败原因加上下文 |

描述要写清"看到这种形态该怎么办"。例如"路径不存在时返回 `path_not_found`，改正路径后重试一次"。

### 2.4 截断要说明丢了多少

`truncated` 布尔只说"被砍了"。模型不知道该重读还是该放弃。

工具结果的 `meta` 追加三个字段：

| 字段 | 含义 |
|---|---|
| `original_bytes` | 原始长度 |
| `truncated_bytes` | 被砍掉的长度 |
| `artifact_path` | 完整内容落盘路径，没有则省略 |

- 出处：ZCode `apps/zcode-cli/packages/core/src/tool/handlers/` 的 `resultBudget` 策略，见 `zcode/tools.md:233-241`

### 2.5 空输出要占位

命令没有输出是正常结果，不是缺失结果。

空输出时正文写 `(no output)` 之类的占位。不要留空串。留空会让模型以为工具没跑。

- 出处：ZCode `apps/zcode-cli/packages/core/src/tool/handlers/`，见 `zcode/tools.md:223-229`

## 3. When to use 与 When NOT

"什么时候用"和"什么时候不要用"必须成对给出。

```text
Use this tool to read a file inside the workspace.
Do not use this tool for directory listings — use the list tool.
Do not use this tool to read more than 500 lines at once — use the search tool to narrow the range first.
```

第二条和第三条是重点。

单工具反例："单事实查询直接搜，不要委派"。读取工具反例："不要读 30 行碎片"。监控工具反例："每一行标准输出都是一次主代理唤醒"。

把 token 成本和交互成本写进工具合同，模型行为会立刻改善。

- 出处：ZCode `apps/zcode-cli/packages/core/src/tool/handlers/agent.ts:111`；MiMo-Code `packages/opencode/src/session/prompt/tool/read.txt:13`；grok-build `crates/codegen/xai-grok-tools/src/implementations/codex/monitor/tool.rs:30-36`

## 4. 好坏对照

难以言明的判断只能靠具象化。给高质量和低质量各一组例子，再收一句"只写高质量的"。

```text
Good: "把 `file_read` 换成 `file_write` 会覆盖原文件。"
Bad:  "这个函数好像有点问题。"
```

- 出处：codex `codex-rs/models-manager/prompt.md:149-196`；claude-code-main `src/tools/TodoWriteTool/prompt.ts:29-121`

示例按当前 schema 条件渲染。参数签名不匹配的示例不插入。

- 出处：deepseek-harness `packages/core/tools/src/ts-types.ts:270-282`

## 5. 工具纪律节

`guidelines` 数组在渲染时拼进静态轨的 `tool_discipline` 节。

```text
# Tool guidelines

- Prefer the dedicated file and search tools over shell equivalents. ...
- Independent tool calls in one response run in parallel. ...
- Reference code as file_path:line_number.
- Read before you edit. ...
- Keep every file operation inside the workspace. ...
- file_read: read the whole region you need before editing it; do not read 30 lines at a time and stitch them together.
- file_write: pass expected_sha256 when you have just read the file; a mismatch means someone else changed it.
- shell_execute: each line of standard output is a round trip to you. Redirect long output to a file and read the file instead.
```

纪律节有两条硬约束。

**一，工具不在场，段自动消失。** 工具集合变化时，`tool_discipline` 节重新渲染。工具被卸载后，它贡献的条目不再出现。

**二，纪律不重复描述。** 如果一条纪律已经写在工具的 `description` 里，就不要在纪律节再写一遍。

## 6. 工具名不写死

工具名在 prompt 中通过常量或占位引用。

重命名工具时，prompt 跟着变，不需要人工核对各处文本。

- 出处：claude-code-main `src/constants/prompts.ts` 的 `${GLOB_TOOL_NAME}`；grok-build `types/template_renderer.rs:48-49`

本项目的做法是：纪律节按在册工具列表动态渲染。条目由工具提供方注册，渲染时用注册时的名字。工具名不写进任何手写文本。

## 7. 工具暴露面

工具数量膨胀会挤占上下文。基准工程用暴露面三态治理。

| 状态 | 含义 |
|---|---|
| direct | 直接进工具表，占请求名额 |
| deferred | 不进工具表，通过检索工具按需发现 |
| hidden | 对模型不可见 |

映射保证 direct 与 deferred 互斥。一个工具要么占请求名额，要么走检索。

- 出处：codex `codex-rs/core/src/tools/`，见 `codex/tools.md:59-81`

本项目的 v1 策略是全部 direct，但按 `tags` 分组筛选。工具提供方在注册时打 `tags`，模型侧按模式过滤。

暴露面三态是目标契约。实现它需要先扩展工具定义的字段集。

## 8. 验收标准

| 项 | 检查方式 |
|---|---|
| 描述五段齐全 | 每个在册工具的描述包含能力、行为、边界、防绕过、When 双举 |
| 无重复事实 | 描述与纪律节、schema 三者之间无同一事实的两次表述 |
| 数值是插值的 | 描述中的超时与上限数值来自配置，改配置后描述随之变化 |
| 不变量前移 | 每个带约束的参数，其描述里含默认值、上限或唯一性说明 |
| 截断可解释 | 产生截断的工具结果带 `original_bytes` 与 `truncated_bytes` |
| 空输出有占位 | 无输出的命令返回非空正文 |
| 缺席即消失 | 卸载一个工具后，重新渲染的纪律节不含该工具的条目 |
| 名称未写死 | prompt 全文检索不出手写的工具名字符串 |
