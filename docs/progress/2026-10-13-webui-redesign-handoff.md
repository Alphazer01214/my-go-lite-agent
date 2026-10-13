# WebUI 重设计 handoff

**日期**：2026-10-13  
**状态**：设计讨论中；旧 WebUI 专用设计文档已清空/删除，本文件记录当前重新对齐后的思路。  
**范围**：先讨论 `agent + llm-openai + session`，重点是 WebUI 的组成、挂载、运行时驱动与 Session Fact。  
**基线检查**：`go test ./...` 通过。没有开始 WebUI 实现。

---

## 1. 项目目标

项目目标不是做一个固定聊天应用，而是构建一套轻量、可组合、可审计、可替换的 Agent Runtime：

- Host 是薄平台内核；
- 能力以进程外 Plugin 提供；
- 插件之间、插件与 Host 之间走统一 Capability / Frame；
- Session Fact Log 是历史与过程真源；
- UI 是平台组合面和业务组件面，不成为业务真源。

WebUI 遵循同一思路：

> Host 提供 Slot 和平台壳，Plugin 提供自己的 Component，Plugin Runtime 通过 pluginsdk 驱动自己的前端；所有需要恢复的业务历史进入 Session Fact。

---

## 2. 当前已对齐的决定

### 2.1 技术栈

WebUI 完全自研，不引入 Vue、React、Svelte、npm、打包器或 iframe。

固定使用：

```text
HTML
CSS Variables
ES Modules
Custom Elements
Shadow DOM
原生 DOM API
Go HTTP / embedded FS
自研 pluginsdk Web Bridge
```

### 2.2 Component 模型

Component 是插件拥有的原生 Custom Element，统一遵循：

```text
setData(state)
runCommand(command, payload)
connectedCallback()
disconnectedCallback()
```

同时提供两类下行驱动：

- **State**：决定组件当前是什么，可刷新后重建；
- **Command**：决定组件此刻做什么，例如滚动、定位、刷新、折叠。

上行统一使用 Capability Call，不发明独立的 UI 业务 API。

### 2.3 挂载模型

有两种挂载方式：

1. **Host Slot 静态挂载**  
   `plugin.json` 声明 Component、Slot、`mount_id`；Host 启动时统一挂载。

2. **owner Component 内部动态挂载**  
   例如未来 sandbox 的确认卡由 `session-chat` 在自己的 DOM 内创建；不改变 Host Slot。

Component 实例身份统一为：

```text
plugin_name + mount_id
```

`mount_id` 只定位组件实例，不参与业务路由；`session_id`、`turn_id` 等仍是业务关联。

### 2.4 第一阶段页面范围

最小组合是：

```text
agent + llm-openai + session
```

布局：

| Slot | 属主 | 内容 |
|---|---|---|
| `top` | Host | Settings、Plugins 入口 |
| `left` | session | 历史会话 |
| `center` | session | 主聊天 |
| `right` | session | Fact Trace |
| `bottom` | 暂定 | 后续状态区域 |

第一阶段实现目标：

- Host Shell 与五槽；
- `session-history`、`session-chat`、`session-trace`；
- agent 与 llm-openai 的统一 Settings Schema；
- `loop.turn` 一次完整对话；
- 流式 chunk 实时累积；
- 刷新后从 Session Fact 恢复；
- 最终 assistant `message` Fact作为正文真源。

第一阶段暂缓：

- Plugins 依赖图完整视图；
- sandbox 动态确认；
- subagent；
- 对话分支；
- 多浏览器实时同步；
- 远程多用户访问。

### 2.5 Settings 模型

Settings 采用统一 Schema：

- Schema 在 `plugin.json` 中静态声明；
- Value、校验、保存和热生效由 Plugin Runtime 提供；
- Host 统一渲染；
- v1 控件类型只包括 `string`、`secret`、`number`、`boolean`、`enum`；
- 每个插件独立显式保存；
- 保存流程：编辑 → 当前插件保存 → 插件校验 → 持久化 → 热生效；
- 校验失败保留表单输入；
- Secret 不回显明文。

第一阶段设置项：

- `agent`：运行模式、最大步数；
- `llm-openai`：Base URL、API Key、Model；
- `session`：暂无必要设置。

### 2.6 插件开关与错误原则

- 插件开关持久化并立即控制目标插件生命周期；
- 用户意图 `enabled/disabled` 与实际状态 `mounted/degraded/...` 分离；
- 关闭一个被依赖的插件时，不级联卸载依赖它的插件；
- 依赖者继续运行，调用得到可恢复错误，报错后继续尝试；
- 普通网络失败、插件暂时禁用、模型超时、工具失败均可恢复；
- 只有插件进程永久退出、协议损坏或确定性终态属于不可恢复错误。

### 2.7 Fact 术语与流式原则

**Fact 就是日志单元；不引入 Event 概念。**

Session Fact Log：

- append-only；
- 会话内顺序递增；
- 保存会话元数据、chat message、tool call/result、turn、step、stream、错误和终态；
- 可支持未来分支与 subagent 因果关系。

流式输出：

- 每次 `llm.complete(stream=true)` 有独立 `stream_id`；
- 使用通用 `stream_chunk` Fact 保存增量；
- 通过 `stream_start + stream_chunk... + stream_end` 累积全文；
- 流成功结束追加最终 `message` Fact；
- `message` 是最终聊天正文真源；
- 取消或失败但已有文本时，保存不完整 `message`，标记 incomplete 和终态；
- 刷新后通过 Session Fact 重建已有 stream，再继续接收后续状态。

公共 Fact 关联方向：

```text
fact_id
seq
type
turn_id
step_id
stream_id
cause_fact_id
source
```

现有 `seq/type/role/content/tool_calls/tool_call_id/meta/ts` 继续保留。具体字段命名和迁移方式仍需后续讨论，但 `Fact` 术语已固定。

---

## 3. WebUI 宏观结构

```text
Host WebUI
├── Shell
├── Slot
├── Component Loader
├── Config Schema Form
├── Inline Component Resolver
└── Web Bridge
```

```text
Plugin Runtime
├── State
├── Command
└── Capability Handler
```

```text
Plugin Component
├── State 展示
├── Command 响应
└── Capability Call
```

pluginsdk WebUI 方向：

```text
SetState(mountID, state)
SendCommand(mountID, command, payload)
Call(capability, method, payload)
ResolveComponent(tag)
```

具体 API 名称尚未最终确定。

---

## 4. 三个插件在 WebUI 中的职责

### 4.1 session

唯一提供第一阶段业务 Component 的插件：

```text
session-history → left
session-chat    → center
session-trace   → right
```

职责：

- `session-history`：调用 `session.list/info`，展示会话元数据与父子关系；
- `session-chat`：调用 `session.query/derive`，展示消息、累积 stream、发起 `loop.turn/cancel`；
- `session-trace`：读取全部 Fact，展示 turn、step、request、stream、tool、usage、错误等；
- 刷新后由 Fact 重建；
- 未来根据 Fact 在 `session-chat` 内动态挂载 sandbox 等交互 Component。

Session Runtime 是这三个 Component 的唯一直接驱动者。

### 4.2 agent

第一阶段没有顶层业务 Component：

- Settings 中修改运行模式；
- Runtime 负责 turn、step、stream、tool 装配；
- 将用户消息、turn/step、stream、tool、最终结果写入 Session Fact；
- 不直接修改 session 的 Component。

当前代码已有 `chat/tool_calling/coding`，目标是改成明确的“极简 / 普通”装配策略；如何保留 coding 等正交维度仍待讨论。

### 4.3 llm-openai

第一阶段没有顶层业务 Component：

- Settings 中配置 Base URL、API Key、Model；
- Runtime 保持无状态；
- 接收 messages/tools，返回最终结果和 stream chunk；
- 不读取 Session；
- 不直接驱动 Component。

---

## 5. 一次 turn 的宏观数据流

```text
session-chat
→ loop.turn
→ agent
→ session.derive（或后续 context-manager）
→ llm-openai
→ stream_chunk
→ agent
→ session.append(stream_chunk facts)
→ session Runtime
→ session-chat 累积
→ session-trace 展示
→ message fact
```

刷新恢复：

```text
session-chat / session-trace
→ session.query
→ 重建 stream/message
→ 接收后续 State / Command
```

---

## 6. 当前代码基线

### 6.1 已有基础

- `plugins/session/types.go`：已有 append-only `Fact`、`seq/type/meta/ts`；
- `plugins/session/store.go`：已有 `create/append/query/derive/list/info`；
- `plugins/session/derive.go`：已把 Fact 投影为模型消息；
- `plugins/agent/turn.go`：已有 turn/step、session 写入、工具循环、取消；
- `plugins/llm-openai/openai.go`：已有 OpenAI-compatible completion 和 stream chunk；
- `plugins/agent/turn.go`：已有 `llm.chunk → loop.chunk` 的实时旁路；
- `Meta`：已有 `parent_session_id/origin/delegation_depth`。

### 6.2 当前缺口

- `pluginsdk/webui.go` 只有空壳；
- 没有 Host Shell、Slot、Component Loader、Web Bridge；
- 没有 Component 或静态资源；
- 没有统一 Config Schema；
- `internal/host` 没有 WebUI 生命周期和前端桥；
- 无 id 的广播 evt 当前直接丢弃；
- `llm.chunk` 尚未写入 Session；
- `reasoning` 尚未保存；
- `llm_usage` 类型已定义但未写入；
- Fact 尚无稳定的 `fact_id`、`stream_id`、`turn_id/step_id`、因果关联；
- 当前模式命名与“极简 / 普通”目标不完全一致；
- `internal/plugin/manifest.go` 的旧 UI 类型尚未按新思路重新设计。

### 6.3 当前工作区

旧专用 WebUI 设计文档已清空/删除。当前仍有用户工作区中的文档改动和 `docs/tmp-ideas.md`，本 handoff 不覆盖、不恢复这些内容。

---

## 7. 下一步讨论点

以下问题尚未最终拍板：

1. 流式 chunk 的保存粒度与顺序保证；
2. Fact 公共字段的最终 schema 与迁移方式；
3. agent 的 minimal/normal 模式如何与 coding 等维度组合；
4. Plugin Runtime 的 State/Command 具体接口；
5. Host Web Bridge 的传输形态；
6. 单浏览器还是多浏览器实时同步；
7. Plugins 页面第一阶段实现到什么程度；
8. 未来 sandbox 内联 Component 的 Fact 表达；
9. subagent 的 `parent_session_id` 与因果 Fact 关联；
10. 对话分支如何从 Fact 位置建立。

---

## 8. 继续讨论的建议顺序

1. 先定 WebUI Platform：Host Shell、Slot、Loader、Bridge；
2. 再定 pluginsdk 的 State / Command / Call 接口；
3. 再定 session 三组件如何消费 Fact；
4. 最后接入 agent turn 与 llm stream；
5. sandbox、subagent、分支放在最小链路稳定后。
