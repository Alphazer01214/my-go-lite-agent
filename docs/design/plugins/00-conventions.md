# 插件设计约定

本文是所有插件设计稿的共同约定。写插件设计稿之前先读本文。

平台协议在 [protocol.md](../../platform/protocol.md) 与 [manifest.md](../../platform/manifest.md)。术语在 [CONTEXT.md](../../../CONTEXT.md)。约束在 [CONSTRAINT.md](../../../CONSTRAINT.md)。

## 1. 三条硬约束

这三条决定了插件之间能怎么协作。任何设计稿违反其中一条，方案就是错的。

**一，插件之间不直连。** 插件要调用别的能力，就发一次 Call，由 Host 路由。插件不许持有别的插件的进程句柄、私有通道或共享内存。

**二，插件名不参与路由。** 路由键是 `(capability, method)`。Manifest 的 `provides` 是能力声明，不是寻址手段。

**三，Host 不解释 payload。** 平台只理解帧字段。业务语义全部留在插件内部。

- 出处：[composition-model-context-tools-sandbox.md](../composition-model-context-tools-sandbox.md) 第 2 节

## 2. 命名

**插件名。** 小写加连字符，`^[a-z][a-z0-9-]*$`。等于目录名，等于 `plugin.json.name`，等于 `pluginsdk.NewPlugin` 的参数。

**能力名。** 小写加连字符，`^[a-z][a-z0-9-]*$`。能力名是业务能力的名字，与插件名可以相同也可以不同。

**方法名。** 小写加下划线，`^[a-z][a-z0-9_]*$`。完整寻址写 `capability.method`。

**工具名。** `^[a-z][a-z0-9_]{0,63}$`。工具名全局唯一。

**错误码。** `^[a-z][a-z0-9_]*$`。全局唯一。

## 3. 能力命名空间

| 能力 | 归属 | 说明 |
|---|---|---|
| `session` | session 插件 | 事实日志与投影 |
| `llm` | llm-openai 插件 | 模型调用 |
| `loop` | agent 插件 | 轮次编排 |
| `agent` | agent 插件 | 编排器自身配置 |
| `model-context` | model-context 插件 | 上下文组装 |
| `tools` | tools 插件 | 工具注册与分发 |
| `file` | file-tools 插件 | 文件操作 |
| `shell` | shell-tools 插件 | 命令执行 |
| `sandbox` | sandbox 插件 | 权限裁决 |
| `subagent` | subagent 插件 | 子代理委派 |
| `memory` | memory 插件 | 跨会话记忆 |
| `skills` | skills 插件 | 技能索引 |
| `mcp` | mcp 插件 | 外部工具服务器接入 |
| `task` | task 插件 | 任务清单 |
| `web` | web-tools 插件 | 网络检索与抓取 |
| `git` | git-tools 插件 | 版本控制操作 |
| `terminal` | terminal 插件 | 持久终端会话 |

新能力接入时先在本表登记，再写插件设计稿。

## 4. Manifest 约定

```json
{
  "name": "example",
  "version": "0.1.0",
  "protocol": 1,
  "entry": "example",
  "description": "一句话说明",
  "autostart": true,
  "priority": 0,
  "provides": ["example"],
  "consumes": [],
  "requires": [],
  "depends_on": [],
  "commands": [],
  "ui": null,
  "settings": null
}
```

**`provides`。** 声明对外能力名。真正路由以运行时上报的 `(capability, method)` 为准。

**`requires`。** 弱依赖能力名。能力缺失时插件仍可启动，调用时返回可恢复错误。

**`depends_on`。** 硬依赖插件名。只影响启动顺序，不参与路由。

**`consumes`。** 参与流水线的 `(capability, method)`。`priority` 决定链内顺序。

**`autostart`。** 是否作为启动根。

**`settings`。** 只在插件需要用户配置时声明。

## 5. 消费链

插件要在别人的调用链上插一脚，就声明 `consumes`。

```text
1. 取活动实例
2. 成员资格：注册过该键，或声明消费该键
3. 去重与跳数上限
4. 按 priority 降序逐跳传播
```

三条规则：

**同方法同优先级冲突。** 同一个 `(capability, method)` 上，两个消费者用同一个 `priority`，Host 返回 `consume_priority_conflict`。

**缺席是 fail-open。** 声明了 `consumes` 但插件未激活时，该跳被跳过，退到下一跳。

**优先级归属。** `priority` 是插件级单值。同插件在不同链上需要不同位置时，用 `consumes[]` 条目内的 `priority` 覆盖。这个字段是提议状态，尚未落地。

- 出处：[composition-model-context-tools-sandbox.md](../composition-model-context-tools-sandbox.md) 第 3 节

## 6. 错误码

**平台级。** 全平台共用，不归任何插件。

| 错误码 | 含义 |
|---|---|
| `bad_arguments` | 参数缺失、类型错、格式错 |
| `handler_error` | 插件内部错误，没有更具体的码 |
| `call_cancelled` | 调用被取消 |
| `call_deadline_exceeded` | 调用超时 |
| `method_not_found` | 目标方法不存在 |
| `route_failed` | 找不到可用路由 |
| `consume_priority_conflict` | 同键同优先级 |
| `flow_already_proceeded` | 重复调用 `Proceed` |
| `ui_not_mounted` | 目标挂载点不存在或队列已满 |

**插件级。** 带插件语义前缀。插件自己的文档定义它。

前缀约定：

| 插件 | 前缀 |
|---|---|
| file-tools | `file_`、`path_`、`edit_` |
| shell-tools | `cwd_`、`command_`、`process_` |
| tools | `tool_` |
| sandbox | `sandbox_` |
| subagent | `subagent_` |
| memory | `memory_` |
| skills | `skills_` |
| mcp | `mcp_` |
| task | `task_` |
| web-tools | `web_` |
| git-tools | `git_` |
| terminal | `terminal_` |
| session | `workspace_`、`session_` |
| model-context | `context_`、`model_context_`、`prompt_` |
| llm-openai | `llm_` |
| agent | `agent_` |

已实现的错误码与前缀的对照：

| 错误码 | 归属 |
|---|---|
| `bad_arguments` | 平台级 |
| `handler_error` | 平台级 |
| `llm_upstream` | llm-openai |
| `llm_output_truncated`、`llm_content_filtered`、`llm_tool_calls_missing` | agent。见下方说明 |
| `workspace_invalid`、`workspace_mismatch`、`workspace_not_found`、`workspace_required` | session 与 file-tools、shell-tools 共用 |
| `file_io`、`file_too_large`、`line_range_invalid` | file-tools |
| `path_escape`、`path_invalid`、`path_is_directory`、`path_not_directory`、`path_not_found` | file-tools |
| `file_exists`、`file_conflict`、`file_binary`、`edit_not_found`、`edit_ambiguous` | file-tools |
| `cwd_escape`、`cwd_invalid`、`command_not_found`、`env_invalid`、`process_start_failed`、`command_blocked`、`process_timeout` | shell-tools |
| `tool_not_found`、`tool_name_conflict`、`tool_owner_mismatch`、`tool_definition_invalid`、`tool_source_invalid`、`tool_target_reserved`、`tool_target_unavailable`、`tool_result_invalid` | tools |

**待定：`llm_` 前缀重叠。**

上表里 `llm_` 前缀同时属于 llm-openai 和 agent。
agent 的三个码由 Turn 循环判定，描述的是模型响应的形态，不是 llm-openai 自身的失败。
两条路可选：把 agent 的码改成 `agent_` 前缀，或把 `llm_` 前缀登记为两个插件共用。
未决前保持现状，改动会触及错误码契约。

## 7. Wire 类型

契约的 Go 实现归插件自己所有。

每个插件在本地 `wire.go` 定义需要的适配类型。`pluginsdk` 不提供共享的业务契约类型。

插件之间只共享 JSON 字段名，不共享 Go 类型。

文档里写 JSON 字段名，不写 Go 结构体名。结构体索引放在每份设计稿末尾，只作实现参考。

## 8. 工具提供者约定

提供工具的插件按三条规则工作。

**一，启动时全量同步。** 插件在 `Serve()` 之前调用 `tools.sync`，把自己的工具集合一次报全。

**二，目标能力是自己的。** 每个工具的 `target_capability` 必须在本插件 `provides` 里。

**三，风险值自己声明。** `risk` 是 0 到 100 的整数。

| 范围 | 含义 |
|---:|---|
| 0 到 19 | 只读或无副作用 |
| 20 到 49 | 可逆的本地修改 |
| 50 到 79 | 显著的本地修改 |
| 80 到 100 | 任意代码执行或破坏性操作 |

**工具描述三件套。** 工具除了 `description`，还可以提供 `guidelines` 与 `prompt_snippet`。前者是跨调用纪律，后者是一句话工具清单。规格见 [prompts/03-tool-prompts.md](../../prompts/03-tool-prompts.md)。

**Tool Result 包络。** 成功与失败都返回同一形状。

```json
{
  "content": "模型可见文本",
  "is_error": false,
  "truncated": false,
  "meta": {}
}
```

`content` 最多 1 MiB。`meta` 最多 64 KiB。

## 9. Prompt 贡献约定

插件影响模型输入有三条路径。

| 路径 | 谁用 | 规格 |
|---|---|---|
| 静态轨节 | 上下文组装插件 | [prompts/02-system-prompt.md](../../prompts/02-system-prompt.md) |
| 动态轨节 | 上下文组装插件 | [prompts/04-runtime-injection.md](../../prompts/04-runtime-injection.md) |
| 工具描述与纪律 | 工具提供方 | [prompts/03-tool-prompts.md](../../prompts/03-tool-prompts.md) |

插件不自己拼 system prompt。上下文组装插件是唯一的组装入口。

插件提供动态轨内容时，产出一个节对象交给组装插件，不自己写消息。

## 10. 记录与可观测

**模型可见就要记录。** 模型能看到的任何内容，都必须能从会话记录里重建。

**过程事实不污染上下文。** 轮次开始、步骤开始、用量统计这类事实进日志，但不带 `message` 类型，不进入模型输入。

**快照记录实际输入。** 每次模型调用记录实际发送的消息顺序、工具集合和内容哈希。

- 出处：[model-context/communication.md](../../../plugins/model-context/communication.md) 第 3 节

## 11. WebUI 约定

**组件归插件。** 组件标签以插件名加连字符开头。

**插槽。** v1 提供 `left`、`center`、`right`、`bottom` 四个插槽。

**当前实现。** 一个插槽在主页面内只归一个插件。第二个插件声明同一插槽时，它的挂载被跳过。

**目标契约。** 一个插槽容纳多个挂载。插槽变成区域，区域内的挂载按插件名排序。实例身份仍是 `plugin_name + mount_id`。

放宽这条规则需要改 Host Shell 的挂载登记表，不改插件契约。放宽前，新插件的组件只是设计稿。

**状态与命令。** `setData` 传可重建状态。`runCommand` 传临时指令。组件不直接读写插件存储。

**命令不持久化。** 滚动、聚焦、临时动画属于命令。持久记录属于插件存储。

- 出处：[CONTEXT.md](../../../CONTEXT.md) 第 4 节

## 12. 设计稿模板

每份插件设计稿用同一套章节。顺序不变，缺项写"无"。

```text
# 插件名

> 状态：当前实现 / 目标契约
> 涉及能力：...
> 依赖：...

## 1. 职责
## 2. 非职责
## 3. Manifest
## 4. 方法与载荷
## 5. 接缝与调用关系
## 6. 工具契约
## 7. Prompt 贡献
## 8. 记录与持久化
## 9. WebUI
## 10. 错误码
## 11. 配置
## 12. 测试要求
## 13. 验收标准
## 14. 未实现
## 15. 结构体索引
```

## 13. 测试要求

每个插件的测试至少覆盖：

| 项 | 说明 |
|---|---|
| 载荷解码 | 缺字段、错类型、空载荷 |
| 成功响应 | 正常路径的响应形状 |
| 稳定错误码 | 每个错误码都有对应用例 |
| 配置持久化 | 写入、脱敏、失败路径 |
| 事件载荷 | 事件字段与顺序 |
| 纯函数 | 投影与转换函数 |
| 取消路径 | 取消与超时 |
| 降级路径 | 弱依赖缺失时的行为 |

## 14. 验收清单

设计稿交付前逐条自查。

| 项 | 检查 |
|---|---|
| 能力名已登记 | 在第 3 节的能力表里 |
| 错误码全局唯一 | 与第 6 节的平台级码不冲突 |
| 不直连其他插件 | 所有跨插件交互都经过能力调用 |
| 不用插件名寻址 | 文档中没有"按插件名调用"的描述 |
| 状态标注清楚 | 每节区分当前实现、目标契约、未实现 |
| 方法载荷完整 | 每个方法有输入、输出、错误 |
| 图与代码一致 | 图中的模块名与代码目录一致 |
| 链接有效 | 链接指向存在的文件 |

## 15. 与其他文档的关系

| 文档 | 关系 |
|---|---|
| [prompts/](../../prompts/README.md) | 模型侧文本的规格。插件按它贡献内容 |
| [protocol.md](../../platform/protocol.md) | 帧格式与路由 |
| [manifest.md](../../platform/manifest.md) | 清单结构 |
| [plugin-dev.md](../../platform/plugin-dev.md) | 开发流程与打包 |
| [composition-model-context-tools-sandbox.md](../composition-model-context-tools-sandbox.md) | 跨插件组合方案 |
| [pluginsdk.md](../../platform/pluginsdk.md) | 插件开发入口 |
