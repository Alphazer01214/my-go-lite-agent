# tools 通信契约

本文定义 `tools` 的跨插件 JSON 契约。

## 1. 方法

| 方法 | 用途 |
|---|---|
| `tools.register` | 注册或更新 Tool |
| `tools.unregister` | 注销本来源 Tool |
| `tools.list` | 列出可用 Tool Definition |
| `tools.call` | 按模型名称分发调用 |

## 2. Tool Definition

```json
{
  "name": "file_read",
  "description": "Read a workspace file",
  "input_schema": {"type": "object", "properties": {"path": {"type": "string"}}},
  "risk": "low",
  "source_plugin": "file-tools",
  "target_capability": "file",
  "target_method": "read"
}
```

`name` 匹配 `^[a-z][a-z0-9_]{0,63}$`。`risk` 只能是 `low`、`medium` 或 `high`。

`source_plugin` 由 Host 注入的 `from` 生成，注册请求不能设置。`target_capability` 和 `target_method` 不发送给模型。

## 3. 注册

输入：

```json
{
  "name": "file_read",
  "description": "Read a workspace file",
  "input_schema": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]},
  "target_capability": "file",
  "target_method": "read",
  "risk": "low"
}
```

输出：

```json
{"registered": true, "updated": false}
```

同来源同名注册为更新。跨来源重名返回 `tool_name_conflict`。

## 4. 注销

```json
{"name": "file_read"}
```

```json
{"unregistered": true}
```

不存在的名称返回 `unregistered: false`。注销其他来源的 Tool 返回 `tool_owner_mismatch`。

## 5. 列表

输入为空对象。输出按 `name` 升序排列。注册方不是 `active` 时隐藏条目。

## 6. 调用

输入：

```json
{
  "name": "file_read",
  "arguments": {"path": "README.md"},
  "workspace": "E:/project",
  "call_id": "call-1"
}
```

Provider 收到的 payload：

```json
{
  "arguments": {"path": "README.md"},
  "workspace": "E:/project",
  "call_id": "call-1"
}
```

调用使用同一个 Go Context 和 `context_id`。Provider 必须返回统一 Tool Result：

```json
{
  "content": "模型可见文本",
  "is_error": false,
  "truncated": false,
  "meta": {}
}
```

## 7. 错误码

| 错误码 | 场景 |
|---|---|
| `tool_not_found` | 名称不存在 |
| `tool_name_conflict` | 跨来源重名 |
| `tool_owner_mismatch` | 注销非本来源 Tool |
| `tool_definition_invalid` | Definition 字段不合法 |
| `tool_source_invalid` | 来源不活跃或不提供 Target Capability |
| `tool_target_reserved` | Target 是 `host` 或 `tools` |
| `tool_target_unavailable` | Target 属主不可用 |
| `tool_result_invalid` | Provider 返回非法结果 |
