# sandbox 通信契约

## 1. 消费方法

| Capability | Method | 风险 |
|---|---|---|
| `tools` | `call` | 按 Tool 与 Target 解析 |
| `file` | `write`、`replace`、`patch`、`move`、`delete` | high |
| `shell` | `execute` | high |
| `web` | `fetch`、`search` | medium |

Consumer 注册同一方法，Priority 为 500。工具层和 Provider 层各检查一次。

## 2. Proceed

允许时调用一次 `Request.Proceed`，原样传递 payload。拒绝时返回协议错误 `sandbox_denied`。

Sandbox 不修改 payload，不执行具体操作。

## 3. Tool Call 解析

`tools.call` 通过 `tools.list` 查找 Tool，合并 Tool 自报风险与 Target 静态风险，取较高者。Tool 未注册、列表失败或风险未知时拒绝。

## 4. 规则

```json
{
  "capability": "file",
  "method": "write",
  "tool": "file_write",
  "action": "allow",
  "workspace_prefix": "E:/project"
}
```

`capability`、`method`、`tool` 和 `workspace_prefix` 均可省略。`action` 只能是 `allow` 或 `deny`。

## 5. 错误

| 错误码 | 场景 |
|---|---|
| `sandbox_denied` | 策略拒绝 |
