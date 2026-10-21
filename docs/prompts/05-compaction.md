# 压缩与摘要

> 状态：目标契约

长会话必然超出上下文窗口。压缩是让会话能继续的手段。

## 1. 两条路径

| 路径 | 机制 | 状态 |
|---|---|---|
| 确定性摘要 | 抽取旧消息的角色与正文首段，截断到上限 | 当前实现 |
| 模型摘要 | 用一次辅助调用生成结构化交接摘要 | 目标契约 |

确定性摘要不丢失意图结构，但会丢失细节。它保证压缩一定成功，代价是信息损失。

模型摘要保留意图、决策和待办，代价是要额外一次调用，且可能失败。

v1 保留确定性摘要作为兜底。当模型摘要不可用或被判为不合格时，退回确定性摘要。

## 2. 压缩 prompt 骨架

模型摘要用固定骨架。骨架固定，节可缺，节不删。

```text
CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.
- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM
that will resume the task. Output EXACTLY the Markdown structure below: keep every section, in
order. Use terse bullets, not prose paragraphs. Write "(none)" for an empty section — never drop
a section.

## Primary Request and Intent
- [the user's original and evolving goals; quote verbatim where the exact wording matters]
## Key Technical Concepts
- [technologies, frameworks, patterns, and conventions in play]
## Files and Code
- [exact path: why it matters, key changes or snippets]
## Errors and Fixes
- [error: how it was resolved, plus related user feedback]
## Pending Tasks
- [explicitly requested work not yet completed]
## Current Work
- [precisely what was in progress at this checkpoint]
## Next Step
- [the single next action, or "(none)"]
## Critical Context
- [decisions and their rationale, constraints, user preferences, open questions]

Rules:
- Preserve exact file paths, function names, and error messages.
- When you encounter an existing <compacted-summary> block, it is a PRIOR checkpoint: preserve
  still-true facts, drop stale ones; do not copy it forward verbatim.
- Do NOT mention this summarization request in the summary.

REMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by
a <summary> block. Tool calls will be rejected and you will fail the task.
```

骨架的三个来源：

| 部分 | 出处 |
|---|---|
| 九节骨架与思考段分离 | claude-code-main `src/services/compact/prompt.ts:66-77` |
| `EXACTLY`、`(none)`、禁丢节、既有检查点合并、保密令 | deepseek-harness `packages/core/compaction/src/summarizer.ts:33-67` |
| 首尾禁令夹层 | claude-code-main `src/services/compact/prompt.ts:12-26`；ZCode `compact/prompt.ts:1-12` |

## 3. 意图必须逐字保留

第一条 `Primary Request and Intent` 要求逐字引用用户原话。

摘要丢意图是长程 agent 的头号故障。模型读到一段概括，会当成一个更窄的任务继续做。

- 出处：MiMo-Code `session/checkpoint/checkpoint-writer.txt:118-121`

## 4. 思考段不入库

模型输出的 `<analysis>` 块提高摘要质量，但不应该进入上下文。

入库前剥掉 `<analysis>`，只留 `<summary>`。

- 出处：claude-code-main `src/services/compact/prompt.ts:311-335`

## 5. 恢复消息

压缩完成后，向会话插入一条恢复消息。

```text
This session is being continued from a previous conversation that ran out of context. The
summary below covers the earlier portion.
Resume directly — do not acknowledge the summary, do not recap, do not re-plan what was already
decided.
```

恢复消息要求模型直接续作。不致谢，不复述，不重做计划。

- 出处：claude-code-main `src/services/compact/prompt.ts:345-350`；deepseek-harness `packages/core/compaction/src/summarizer.ts:70-71`

## 6. 迭代压缩

再次压缩时，输入里已经有上一份检查点。

合并规则：保留仍然成立的事实，丢弃已失效的事实。不要把上一份摘要原样抄过来。

- 出处：deepseek-harness `packages/core/compaction/src/summarizer.ts:66`

## 7. 拒绝坏摘要

| 情形 | 处理 |
|---|---|
| 辅助调用以错误结束 | 不落库，保持旧摘要 |
| 辅助调用因长度截断 | 不落库，保持旧摘要 |
| 摘要长度不短于被压缩的内容 | 判为失败，退回确定性摘要 |
| 摘要缺少骨架中的任一节 | 判为失败，退回确定性摘要 |
| 摘要中出现工具调用 | 判为失败，退回确定性摘要 |

- 出处：pi `coding-agent/src/core/compaction/compaction.ts:607-614`；deepseek-harness-x `packages/compaction/compaction-basic/README.md`

## 8. 触发点

| 触发点 | 条件 | 状态 |
|---|---|---|
| 预算触发 | 估算用量超过窗口的压缩阈值 | 当前实现 |
| 轮次开始前 | 上下文预算不足一次请求 | 目标契约 |
| 轮次中途 | 单步输出过长，需要腾出空间 | 目标契约 |
| 手动 | 调用方显式要求压缩 | 当前实现 |

压缩只覆盖普通对话消息。system prompt、运行时上下文不受影响。

压缩按用户消息分段，不切开工具调用与工具结果的配对。

- 出处：上下文组装插件当前实现的分段规则，见 [docs/design/plugins/04-model-context.md](../design/plugins/04-model-context.md)

## 9. 成本技巧

压缩指令不作为独立的 system prompt 发出。

它作为"重放对话前缀之后的最后一条 user 消息"发出。这样辅助调用与主请求共享前缀，可以命中服务端的提示缓存。

- 出处：deepseek-harness-x `packages/compaction/compaction-basic/src/summarizer.ts:25-30`；deepseek-harness 同款

## 10. 主 prompt 的披露

主 system prompt 提前说明压缩存在。

```text
- The runtime compresses older messages automatically when the context budget is reached, so the session is not limited by the context window.
```

不披露的话，模型会担心上下文被截断而提前收尾。

- 出处：claude-code-main `src/constants/prompts.ts:193`

## 11. 验收标准

| 项 | 检查方式 |
|---|---|
| 骨架完整 | 落库摘要含全部九节，缺节判失败 |
| 意图逐字 | 摘要第一节含用户原话的逐字片段 |
| 思考段剥离 | 落库摘要不含 `<analysis>` 标记 |
| 坏摘要不落库 | 错误、截断、超长、缺节四种输入都保留旧摘要 |
| 合并正确 | 二次压缩后摘要不含已失效事实 |
| 配对不切开 | 摘要边界不落在工具调用与结果之间 |
| 预算不越界 | 压缩后估算用量低于上限，否则报预算超限 |
