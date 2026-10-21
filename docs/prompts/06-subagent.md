# 子代理画像

> 状态：目标契约

子代理有独立会话、自己的工具面、自己的画像。

## 1. 画像三段式

子代理的画像由三块组成，缺一不可。

```text
You are a <role> subagent — a focused worker delegated a specific task. The parent agent, not
you, communicates with the end user.

=== CRITICAL: <MODE — e.g. READ-ONLY, NO FILE MODIFICATIONS> ===
You are STRICTLY PROHIBITED from:
- <逐条列举，含用重定向或 heredoc 写文件这类容易被漏掉的旁路>
You do NOT have access to <mutation tools> — attempting to use them will fail.

<strengths / guidelines：这个角色擅长什么，按什么顺序干活>

<输出契约：>
Report outcomes faithfully. If part of the task is impossible or contradicted by what you found,
say so instead of filling a field with a plausible guess. Never fake a passing result.
Ground every claim in something you read or ran in this session, and say which. Cite code as
path:line.
```

- 出处：claude-code-main `src/agents/exploreAgent.ts:24-56`、`src/agents/verificationAgent.ts:10-81`；ZCode `apps/zcode-cli/packages/core/src/context/sections/workflow-actor.ts:33-46`

三段各管一件事。

| 段 | 作用 | 写错的后果 |
|---|---|---|
| 身份 | 让子代理知道自己是谁、替谁干活 | 越权替用户做决定 |
| 禁令 | 列出绝对不能做的事 | 越权写文件 |
| 输出契约 | 规定回报的形态与诚信标准 | 报告失真 |

禁令要逐条列举，并点名容易被漏掉的旁路。只写"不要修改文件"不够，模型会用重定向绕过。

- 出处：claude-code-main `src/agents/exploreAgent.ts:24-56`

## 2. 父代理的委派元指令

这段放进委派工具的描述，教父代理写好委派说明。

```text
## Writing the prompt

Brief the agent like a smart colleague who just walked into the room — it hasn't seen this
conversation, doesn't know what you've tried, doesn't understand why this task matters.
- Explain what you're trying to accomplish and why.
- Describe what you've already learned or ruled out.
- Give enough context that the agent can make judgment calls rather than following a narrow
  instruction.
- If you need a short response, say so ("report in under 200 words").
- Lookups: hand over the exact command. Investigations: hand over the question — prescribed
  steps become dead weight when the premise is wrong.

Never delegate understanding. Don't write "based on your findings, fix the bug" — that pushes
synthesis onto the agent instead of doing it yourself. Include file paths, line numbers, what
specifically to change.

A new agent call starts fresh: the prompt must be self-contained.
```

- 出处：claude-code-main `src/tools/AgentTool/prompt.ts:99-113`；ZCode `apps/zcode-cli/packages/core/src/tool/handlers/agent.ts:114`

三条要点。

**委派说明必须自足。** 子代理可能看不到父会话。

**不委派理解。** "根据你的发现修这个 bug"把综合推给了子代理。父代理要给出文件、行号、改什么。

**查和查证是两种委派。** 查证给问题，不给步骤。步骤在前提错误时会变成死重。

## 3. 继承与独立

子代理是否继承父对话，影响工具描述的措辞。

```text
inherits: It already sees this conversation's completed turns, so build on them freely and
          state only what is new.
fresh:    It does not share this conversation's context, so include everything it needs.
```

两种模式必须用不同措辞。描述与实际行为不一致会直接导致错误委派。

- 出处：deepseek-harness-x `packages/subagent/tool-subagent/src/index.ts:211-236`

## 4. 公共注记

绝对路径规则、禁用表情符号、工具调用前不加冒号这类约定，由统一函数追加。

画像只写"你是谁"。公共注记由平台注入，不允许画像覆盖。

- 出处：ZCode `apps/zcode-cli/packages/core/src/context/sections/subagent/system-prompt.ts:10-18`

## 5. 权限冻结

子代理的权限范围在启动时固定。

```text
Your permission scope was fixed when you were started and cannot be widened from inside this session.
```

这句话把"被拒后不要重试"变成协作协议。子代理发现自己权限不足时，应该报告，而不是换路再试。

- 出处：deepseek-harness `packages/core/agent/src/child-agent.ts:171-176`

## 6. 深度限制

深度限制写两处。

| 位置 | 作用 |
|---|---|
| 委派侧 | 发起时检查，超过上限直接拒绝 |
| 执行侧 | 子代理按会话父链自算实际深度 |

只写委派侧不够。调用方可以伪造深度字段。

- 出处：codex `codex-rs/core/src/agent_control/`，见 `codex/subagent.md:83-85`；ZCode `apps/zcode-cli/packages/core/src/subagent/`，见 `zcode/subagent.md:256`

## 7. 工具面先锁，证据标准后提

零工具的子代理配上"必须给出证据"的要求，会逼出退化的占位式回复。

修法是在画像里把工具面说死，再提证据标准。

- 出处：ZCode `apps/zcode-cli/packages/core/src/context/sections/workflow-actor.ts:13-17`

## 8. 与主 prompt 的关系

子代理不另写一套主 system prompt。

它复用同一份静态轨，追加自己的画像节。这样同一个部署下的所有代理共享同一套纪律。

跨位置重复是必要的。主 prompt 里有的关键规则，在子代理画像里再写一遍。子代理不一定看得到主 prompt。

- 出处：claude-code-main `src/constants/prompts.ts:438` 与 `:770`；rea `src/contracts/promptContracts.ts:84`

## 9. 验收标准

| 项 | 检查方式 |
|---|---|
| 三段齐全 | 每个子代理画像含身份、禁令、输出契约 |
| 禁令可判定 | 禁令逐条列举，含至少一条容易被漏掉的旁路 |
| 委派说明自足 | 子代理提示不依赖父会话内容即可执行 |
| 不委派理解 | 委派模板明确禁止"根据你的发现修"这类写法 |
| 措辞与行为一致 | 继承模式与独立模式的描述文字不同且与实现一致 |
| 权限冻结 | 画像含权限范围固定的声明 |
| 深度双检 | 委派侧与执行侧都做深度检查 |
