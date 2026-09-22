# Protocol — Host ↔ Plugin 通信协议（v1）

本文是 **plugin-host / plugin-plugin 通信协议** 的唯一真源。规范名词见 [CONTEXT.md](../CONTEXT.md)。清单字段说明见 [manifest.md](manifest.md)。

**寻址模型**：路由与 dispatch **只看 `capability` + `method`**。插件名仅供日志、依赖图、观测，**不参与路由**。

**v1 非目标**（有意不做，勿从本文推断已有）：调用超时、生命周期状态机、按需挂载/开关、evt 广播扇出、SDK 异步/流式 API、一切 UI / HostFace / 呈现面。

---

## 1. 传输与 Frame

### 1.1 传输

| 项 | 约定 |
|----|------|
| 拓扑 | 星型：插件 ↔ Host ↔ 插件；**插件之间不直连** |
| 通道 | 子进程 stdio：Host 写插件 **stdin**，读插件 **stdout** |
| stderr | 不走协议（日志） |
| 帧编码 | **4 字节大端无符号长度** + **UTF-8 JSON body** |
| 长度上限 | `16 << 20`（16 MiB），超限拒绝读写 |
| 字节序 | 长度前缀固定 big-endian |

### 1.2 Frame 字段（snake_case）

一条 Frame 即一条完整消息。JSON 字段：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `version` | int | 是 | **线协议**版本；v1 恒为 `1` |
| `id` | string | 是 | 请求/响应闭环 ID；无归属的 evt 可为空串 |
| `type` | string | 是 | `req` \| `res` \| `evt` |
| `capability` | string | req 必填 | 能力名；与 `method` 合成路由键 |
| `method` | string | req 必填 | 方法名；路由键的另一半 |
| `from` | string | 投递时必有 | 来源插件名；**由 Host 注入**，插件填写无效。**每次 Call/投递都带上**，只用于溯源、日志、依赖图，**不参与路由** |
| `payload` | object/array/… | 否 | 不透明 JSON；Host **不解读**业务内容 |
| `error_code` | string | 是 | 错误码；**成功时为空串** |
| `error_msg` | string | 是 | 错误信息；**成功时为空串** |

说明：

- **无 `to` 字段**。目标由 Host 查 **`(capability, method)` 路由表** 决定。
- **`from` 必带**：Host 在每次投递/转发时写入真实来源插件名，保证每条 Call 可溯源；调用方不需要也无法伪造。
- **每个消息都带** `error_code` / `error_msg`（可为空串）。**不使用**嵌套的 error 对象。
- 成功：`error_code == ""` 且 `error_msg == ""`，结果放在 `payload`。
- 失败：`error_code != ""`，`error_msg` 为可读信息；`payload` 不作为结果。
- 错误语义属于 **`res` 闭环**。正常路径上 `req` / `evt` 的错误字段必须为空。
- 错误主要在**插件内**处理（见 §4）；Frame **流经 Host** 时若发现错误字段非空，Host **只记日志**，不改写、不拦截业务错误。

### 1.3 版本（双层）

| 层 | 字段 | 管什么 |
|----|------|--------|
| 线协议 | Frame `version` | 帧字段/编码/路由语义 |
| 清单契约 | Manifest `protocol` | `plugin.json` 契约（见 manifest.md） |

二者独立演进。破坏线格式 → 升 Frame `version`；破坏清单契约 → 升 Manifest `protocol`。Host 发现 Frame `version` 不支持时拒绝该连接（合成 `res` 或关闭进程，记日志）。

### 1.4 ID 规则

| 产生方 | 形式 | 说明 |
|--------|------|------|
| 插件发出的 req | `<plugin_name>-<seq>` | 如 `session-1`；SDK 在进程内递增 |
| Host 转发改写 | `fwd-<seq>` | Host 分配；对被调方可见 |
| Host 主动发起 | `host-<seq>` | Host 作为调用方时 |
| res | 与对应 req **同 id** | 被调方回 `fwd-N`；Host 还原成调用方原始 id 后再写回 |

Host 维护 `pending[fwd-N] → {caller, original_id, …}` 以完成改写与归因。`plugin_name` 出现在 id 前缀里仅便于排查，**不是**路由输入。

---

## 2. 三类消息

### 2.1 Request（`type=req`）

- **功能**：要求对端执行 `capability.method`，并**恰好**对应一条 `res`。
- **寻址**：`capability` + `method` 必填；Host 查路由表得到唯一属主进程后转发。
- **允许自调用**：属主即 `from` 时仍合法，Host **环回**投递给该插件（统一 pending/日志；不本地短路）。
- **产生**：插件经 `pluginsdk.Call`；Host 经内部主动调用（`host-N`）。
- **消费**：路由表中的属主插件；`pluginsdk.Serve` 按 `capability.method` 分发给 `Register` 的 `handleFunc`。
- **错误字段**：正常为空。Host 在路由失败时**合成** `res`（见 §3.3）并填错误码。

### 2.2 Response（`type=res`）

- **功能**：对某个 `req` 的唯一闭环。
- **寻址**：无需路由键；Host 按 `id` 找到 pending 后写回 **caller** 进程。
- **产生**：被调插件（`handleFunc` 返回后由 SDK 写出）；或 Host 合成（路由失败/对端死亡）。
- **消费**：发起 Call 的插件（SDK 按 id 完成 pending）。
- **成功**：`error_code == ""`，结果在 `payload`。
- **失败**：`error_code != ""`，`error_msg` 非空；插件可用 `pluginsdk.ErrCode` 指定细分码，否则默认 `handler_error`。

### 2.3 Event（`type=evt`）

- **功能**：不要求应答的旁路。用于过程信息、流式片段、状态推送等。**不建立** req/res 闭环。
- **字段**：与 Frame 全集相同；关键差异：
  - `id`：见两种细类；
  - `capability` / `method`：主题（如 `presentation.stream`），**不是**点对点路由目标；
  - `payload`：不透明；
  - `error_code` / `error_msg`：正常为空（失败语义仍属 res；插件内部处理）。

**两种细类：**

| 细类 | `id` | 语义 | Host v1 行为 |
|------|------|------|----------------|
| 归因 evt | 非空，指向在途 req | 属于某次 Call（如 LLM 流式 delta） | 查 `pending`：命中则挂到该次调用的旁路事件列表；未命中则 log 后丢弃 |
| 广播 evt | 空 | 无指定接收调用的旁路推送 | **广播扇出未实现**：仅 log 后丢弃 |

**产生 / 消费位置：**

| 角色 | 产生 | 消费 |
|------|------|------|
| 插件 | 业务过程、流式、状态（线协议已支持） | — |
| Host | 可合成诊断/生命周期 evt（v1 可不使用） | 归因挂载、日志 |
| 调用方插件 | — | 归因 evt（v1 SDK 不暴露读取 API，见下） |
| 其它订阅方 | — | 广播 evt（**未实现**） |

**SDK 面（v1）**：不暴露 `Emit` / `EmitTo`。evt **字段与语义以本文为准**；插件若手写帧发出 evt，Host 按上表处理。

---

## 3. 路由与转发（Host 内部）

### 3.1 原则

1. **路由键** = `(capability, method)`。全局至多一个属主插件进程。
2. **插件名不参与路由**。`from` 只作观测；查找、转发、闭环均不按名字寻址业务目标。
3. `payload` 对 Host **不透明**（`capability=host` 的横切方法除外，见 §5）。
4. 一切跨插件通信经 Host 中转；Host 不实现插件间直连。

### 3.2 路由表

| 项 | 约定 |
|----|------|
| 键 | `(capability, method)` |
| 值 | 属主插件进程（内部可用插件名索引进程，**不对调用方暴露寻址语义**） |
| 唯一性 | 全局唯一属主；冲突见下 |
| 填充 | 插件 `Serve()` 启动时向 Host 上报本进程已 `Register` 的键集合（§5.1 `register`） |
| 查表失败 | 合成 `res`，`error_code=method_not_found` |

**冲突规则（Register 语义）：**

| 情形 | 行为 |
|------|------|
| **同一插件**重复 `Register` 同一 `(capability, method)` | **last-wins**（覆盖本地 handler） |
| **另一插件**再上报同一 `(capability, method)` | **拒绝**该键：Host 不更新属主；上报方对应键不生效（`capability_conflict`） |

### 3.3 入站检查链（`req`）

对来自插件的 `req`，Host 依次：

1. **host 已关闭** → 合成 `res`，`error_code=host_closed`。
2. **`capability` 或 `method` 为空** → 合成 `res`，`error_code=method_not_found`（缺路由键即无法寻址）。
3. **`capability == "host"`** → 进入 §5 host 方法分派（不查业务路由表）。
4. **路由表无此 `(capability, method)`** → 合成 `res`，`error_code=method_not_found`。
5. **属主进程未挂载 / 不可用** → 合成 `res`，`error_code=plugin_not_mounted` 或 `plugin_down` / `plugin_disabled`。
6. 否则 **forward**（含属主 == `from` 的环回）：分配 `fwd-N`，登记 pending，改写 `id`、注入 `from`，写入属主 stdin。

v1 挂载策略为「发现即启动」，一般不会出现「已发现但未挂载」；仍保留错误码以兼容后续生命周期。

### 3.4 Response / 死亡闭环

- 被调方 `res`：Host 按 `id` 查 pending，**还原原始 id**，`from` 设为被调插件名（观测用），写回 caller；删除 pending。
- **被调进程退出 / 读失败**：Host 对与该属主相关的 pending **合成** `res`，`error_code=plugin_down`（或 `server_closed`），并写回 caller。
- **每个 req 最终有且只有一条 res**（对端实现或 Host 合成）。
- v1 **无超时**：对端不退出且不回包时 Call 可一直阻塞；进程死亡必须闭环（见上）。

### 3.5 `from` 注入与溯源

- 插件出站帧的 `from` 可为空；**Host 在投递/转发时必须写入真实来源插件名**，不可被插件伪造。**每一次 Call 相关的投递都带上 `from`**，供日志、审计、依赖图回放「谁在何时调了什么」。
- Host 自己发出的帧：`from = "host"`。
- **禁止**任何组件用 `from` 或插件名做业务路由选择（溯源 ≠ 寻址）。
- id 前缀 `<plugin_name>-<seq>` 同样只服务排查，不是路由输入。

### 3.6 Event 路由（v1）

- 带 `id`：与 pending 匹配则追加旁路事件；否则 log。
- 无 `id`：log 后丢弃（不做扇出）。
- 不修改归因 evt 的 `id`。

```mermaid
sequenceDiagram
    participant A as Plugin A
    participant H as Host
    participant B as Plugin B
    Note over A,B: 路由键 = capability.method（无 to）
    A->>H: req id=session-1 cap=llm method=complete
    H->>H: owner[(llm,complete)]=B; pending[fwd-1]=session-1
    H->>B: req id=fwd-1 from=A cap=llm method=complete
    B-->>H: evt id=fwd-1（归因，可选）
    H-->>A: （v1 SDK 不读旁路，仅 Host 挂 pending）
    B->>H: res id=fwd-1 payload / error_code
    H->>A: res id=session-1 from=B
    H->>H: delete pending[fwd-1]
```

---

## 4. pluginsdk（插件开发者接口）

插件作者经本 SDK 说话，**不要**手写 Frame 循环（夹具除外）。

### 4.1 进程身份

```text
pluginsdk.New(name string) *Server
```

- `name` 为**插件名**，进程创建时声明一次，应与目录名 / `plugin.json` 的 `name` 一致。
- 仅用于日志、依赖图、`from` 观测与 id 前缀；**不用于路由**。
- 之后仅注册 handler；**Register 不带 name**。

### 4.2 注册

```text
Register(capability, method string, h handleFunc)

handleFunc func(payload json.RawMessage) (result json.RawMessage, err error)
```

- 登记本进程的 `capability.method` handler。
- **同一插件**内重复注册同一键：**last-wins**（覆盖并建议日志）。
- 跨插件属主冲突不在本地判：以 Host `register` 上报结果为准（§3.2 / §5.1）。
- 未登记的入站 `req`：SDK 回 `res`，`error_code=method_not_found`。
- `err == nil`：成功，`result` 为 `payload`，错误字段为空。
- `err != nil`：失败；`error_code` 取 `pluginsdk.Code(err)`，空则 `handler_error`；`error_msg=err.Error()`。

**指定细分错误码：**

```text
pluginsdk.ErrCode(code, msg string) error
pluginsdk.Code(err error) string   // 无 code 的普通 error 返回 ""
```

调用方按 code 分支：`if pluginsdk.Code(err) == "method_not_found" { … }`。不在线格式中引入 error 结构体。

### 4.3 调用

```text
Call(capability, method string, payload json.RawMessage) (json.RawMessage, error)
```

- 同步：发 `req` 并阻塞至对应 `res` 或本地错误。
- **只点名 `capability.method`**，不点名插件；Host 查表转发到唯一属主（允许属主即自己，环回）。
- **溯源**：Host 必定在出站/回程帧上注入 `from`（及 id 前缀中的插件名），每条 Call 都能回答「谁发起」。
- 成功：返回 `payload`，`error == nil`。
- 失败：`error` 带 code（Host 合成码或对端 `handleFunc` 码）。

### 4.4 读循环

```text
Serve() error
```

- 将本进程已 `Register` 的 `(capability, method)` **上报 Host**（§5.1 `register`），然后阻塞读 stdin，分发 `req`，写出 `res`；完成出站 `Call` 的 pending。
- 典型 `main`：`New(name)` → `Register…` →（可选）`Call…` → `Serve()`。

### 4.5 升级为带 ctx 的签名（预留）

v1 不引入 ctx，但协议允许向后兼容演进为：

```text
handleFunc func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)
Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
```

v1 实现可在内部使用 `context.Background()`。届时取消语义与超时可挂在 ctx 上，**不改变** Frame 字段名。

---

## 5. Host 能力（一切皆插件）

Host 内建 Capability **`host`**。调用方只需 `capability=host` + `method=…`，**不使用插件名寻址**。

| 项 | v1 约定 |
|----|---------|
| 路由 | `capability = "host"` → Host 本地分派（不查业务路由表） |
| method | 选 Host 方法；Host 可解析该 `payload`（横切面，允许） |
| `from` | 由 Host 注入真实调用方 |

### 5.1 v1 方法

| method | 功能 | payload | 成功 payload |
|--------|------|---------|----------------|
| `register` | 插件上报本进程 handler 键集合（Serve 启动时） | `{"handlers":[{"capability":"…","method":"…"},…]}` | `{"accepted":[…],"rejected":[…]}`（rejected 含冲突键） |
| `plugins` | 查询当前已挂载插件名列表（观测/依赖图） | `{}` 或空 | `{"plugins":["agent","session",…]}` |

`register` 是路由表的**唯一写入口**。拒绝的键不得再被该进程的 Call 当作已发布能力。

### 5.2 Reserved（名字保留，行为非目标）

下列 method 在协议中**保留**，v1 可不实现；实现前调用应返回 `method_not_found`：

| method | 意图 |
|--------|------|
| `ensurePlugins` | 幂等挂载 |
| `setPluginEnabled` | 启用/禁用插件 |
| `pluginSwitch` | 持久化启用名单 |

**UI / HostFace 不在 v1 范围**：不定义 `config` / `commands` / `ui` 面，不定义面板/呈现载荷。后续若引入，仍应落成普通 `(capability, method)`，不得再引入按插件名的第二套路由。前瞻设计见 [docs/future/ui-medium.md](future/ui-medium.md)。

---

## 6. 挂载（v1 一句话）

**发现即全量启动**：Host 扫描 `pluginsDir/<name>/plugin.json`，校验目录名与 `name` 一致后启动全部已发现插件进程，不建生命周期状态机、不做 Autostart/DependsOn 闭包、不提供开关。缺失/启动失败：记日志并跳过该项（软失败），Host 不退出。

清单契约见 [manifest.md](manifest.md)：

- `provides`：声明本插件将注册的**能力名**（观测/依赖图；真正路由以 `host.register` 上报的 `(capability, method)` 为准）。
- `requires`：需要的能力名（弱依赖；v1 不强制校验）。
- `depends_on`：硬依赖插件名（仅依赖图/未来装配；**不参与 Frame 路由**）。

---

## 7. 错误码

线格式填在 `error_code`。SDK 用 `Code(err)` / `ErrCode(code, msg)` 对齐下表。

| code | 产生方 | 含义 |
|------|--------|------|
| `host_closed` | Host | 宿主已关闭 |
| `method_not_found` | Host / 插件 SDK | 路由表无此 `(capability, method)`，或插件未注册该 handler，或 host 无此 method |
| `route_failed` | Host | 路由失败（其它） |
| `plugin_not_mounted` | Host | 属主进程未挂载 |
| `plugin_down` | Host | 属主进程不健康/已退出 |
| `plugin_disabled` | Host | 属主已禁用（预留） |
| `server_closed` | Host / SDK | 服务结束导致 pending 失败 |
| `handler_error` | 插件 SDK | `handleFunc` 返回无 code 的错误（默认） |
| `bad_payload` | 插件 / Host | payload 无法解析 |
| `bad_arguments` | 插件 / Host | 参数不合法 |
| `frame_too_large` | 传输层 | 超过 16 MiB |
| `capability_conflict` | Host | `(capability, method)` 已有其它插件属主，`register` 拒绝该键 |
| `ensure_plugins_failed` | Host | 预留 |
| `set_plugin_enabled_failed` | Host | 预留 |

插件可用 `ErrCode` 使用上表之外的业务码；Host 不解释业务码，仅透传并可在流经时记日志。

**已废弃（不得再依赖）**：`to_required`、`call_self`（无 `to`；自调用合法）。

---

## 附录：与实现的对应

| 契约 | 参考实现（演进中） |
|------|-------------------|
| Frame / 读写 | `internal/host/transport.go` |
| 契约常量 | `internal/host/constant.go` |
| Manifest | `internal/plugin/manifest.go` |
| 发现 | `internal/plugin/discovery.go` |
| pluginsdk | `pluginsdk/`（按 §4 实现） |
