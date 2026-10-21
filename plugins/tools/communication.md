# tools 通信契约

本文定义 `tools` 的跨插件 JSON 契约。

契约的 Go 实现归各插件自己所有。`tools`、`file-tools`、`shell-tools` 分别在本地 `wire.go` 定义所需类型，`pluginsdk` 不提供共享工具契约。各插件只共享本文描述的 JSON 字段，不共享 Go 类型。

## 1. 方法

| 方法 | 用途 |
|---|---|
| `tools.sync` | 原子替换本来源的完整 Tool 集合 |
| `tools.register` | 增量注册或更新单个 Tool |
| `tools.unregister` | 注销本来源的单个 Tool |
| `tools.list` | 列出当前可用 Tool |
| `tools.call` | 校验并分发一次工具调用 |

Provider 启动时调用 `tools.sync`。运行期动态增删使用 `register` / `unregister`。

## 2. Tool Definition

```json
{
  "name": "file_read",
  "description": "Read a workspace file.",
  "input_schema": {"type": "object", "properties": {"path": {"type": "string"}}},
  "risk": 10,
  "parallel_safe": true,
  "tags": ["fs", "read"],
  "source_plugin": "file-tools",
  "target_capability": "file",
  "target_method": "read",
  "source_generation": 3
}
```

| 字段 | 说明 |
|---|---|
| `name` | 全局唯一，匹配 `^[a-z][a-z0-9_]{0,63}$` |
| `description` | 1 到 8192 字节 |
| `input_schema` | object schema，最多 64 KiB |
| `risk` | 0 到 100 的整数，越高越危险 |
| `parallel_safe` | 可选。只读或无共享状态时为 `true`，允许 agent 的 `auto` 模式并发 |
| `tags` | 最多 16 个，供 agent 模式筛选 |
| `source_plugin` | Host 注入的 `from`，注册方不能设置 |
| `target_capability` | 目标 Capability，不能是 `host` 或 `tools` |
| `target_method` | 目标方法 |
| `source_generation` | tools 填写，用于识别陈旧工具 |

`source_plugin`、`parallel_safe`、`target_capability`、`target_method`、`source_generation` 和 `tags` 不发送给模型。

### Risk

`risk` 是整数尺度：

| 范围 | 含义 |
|---:|---|
| 0–19 | 只读或无副作用 |
| 20–49 | 可逆的本地修改 |
| 50–79 | 显著的本地修改 |
| 80–100 | 任意代码执行或破坏性操作 |

当前工具取值：只读文件工具 `10`，文件写入 `80`，命令执行 `90`。

## 3. 同步

输入：

```json
{
  "tools": [
    {
      "name": "file_read",
      "description": "Read a workspace file.",
      "input_schema": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]},
      "risk": 10,
      "tags": ["fs", "read"],
      "target_capability": "file",
      "target_method": "read"
    }
  ]
}
```

输出：

```json
{"synced": true, "added": 8, "updated": 0, "removed": 2}
```

- 全量校验通过后才替换；任一 Definition 非法则整批不变。
- 空数组表示注销该来源的全部 Tool。
- `source_plugin` 与 `source_generation` 由 tools 填写。
- Provider 重启后必须重新 `sync`，否则旧工具会被 Generation 判定为陈旧。
- tools 把注册表缓存到插件目录的 `registry.json`。tools 自身重启后先恢复缓存；运行时仍按 Generation 与 active 状态校验。

## 4. 注册与注销

`tools.register` 输入单个 Definition。输出：

```json
{"registered": true, "updated": false}
```

`tools.unregister` 输入：

```json
{"name": "file_read"}
```

输出：

```json
{"unregistered": true}
```

不存在的名称返回 `unregistered: false`。注销其他来源的 Tool 返回 `tool_owner_mismatch`。

注册时校验：

- `req.From` 非空；
- 来源 Manifest 的 `provides` 包含 `target_capability`；
- 名称、schema、risk、tags、target 合法；
- 跨来源重名拒绝，同来源同名更新。

注册不要求来源已经 `active`。Provider 在 `Serve` 前调用 `sync` 时实例仍是 `loading`，但 Generation 已分配。

## 5. 列表

输出按 `name` 升序排列。工具可见需要同时满足：

1. 来源插件存在且启用；
2. 来源 Manifest 提供目标 Capability；
3. 至少一个 `active` Instance 注册了目标方法；
4. 来源当前最大 Generation 与 Tool 记录的 Generation 一致。

任一不满足则隐藏。Generation 不一致表示 Provider 已重启但尚未 `sync`。

## 6. 调用

输入：

```json
{
  "name": "file_read",
  "arguments": {"path": "README.md"},
  "session_id": "s-1",
  "workspace": "E:/project",
  "workspace_id": "ws-1",
  "call_id": "call-1",
  "mode": "coding"
}
```

`arguments` 必须是 JSON object。

Provider 收到：

```json
{
  "arguments": {"path": "README.md"},
  "session_id": "s-1",
  "workspace": "E:/project",
  "workspace_id": "ws-1",
  "call_id": "call-1",
  "mode": "coding"
}
```

tools 使用同一个 Go Context 和 `context_id` 调用 `target_capability.target_method`。目标调用再次进入 Host 流水线，因此 sandbox 等消费者可以拦截。

Provider 返回统一 Tool Result：

```json
{
  "content": "模型可见文本",
  "is_error": false,
  "truncated": false,
  "meta": {}
}
```

- `content` 最多 1 MiB；`meta` 最多 64 KiB。
- 业务失败返回 `is_error: true`。
- 传输失败返回协议错误；agent 会把它转成 tool error。

## 7. 错误码

| 错误码 | 场景 |
|---|---|
| `tool_not_found` | 名称不存在 |
| `tool_name_conflict` | 跨来源重名 |
| `tool_owner_mismatch` | 注销非本来源 Tool |
| `tool_definition_invalid` | Definition 字段不合法 |
| `tool_source_invalid` | 来源未提供目标 Capability |
| `tool_target_reserved` | Target 是 `host` 或 `tools` |
| `tool_target_unavailable` | 来源或目标方法不可用、Generation 陈旧 |
| `tool_result_invalid` | Provider 返回非法结果 |

## 8. 拓扑约束

- tools 只是 `tools.call` 的 Provider，不是文件或命令方法的 Owner。
- Provider 只能注册指向自身 Capability 的 Tool。
- tools 不按插件名寻址，只按 `(capability, method)` 分发。
- tools 不执行策略；sandbox 等消费者在 `tools.call` 或 Provider 方法上独立参与。
