状态：当前实现

# Sandbox

Sandbox 是 Tool 接缝上的裁决 Consumer。它只决定放行、确认、改写或拒绝，不执行文件与命令。

## 拦截点

Sandbox 启动时读取自己的 `plugin.json`，为 `consumes` 里的每个 `(capability, method)` 注册同一个 seam handler。默认清单：

```json
[
  { "capability": "tool", "method": "call" },
  { "capability": "file", "method": "write" },
  { "capability": "file", "method": "edit" },
  { "capability": "shell", "method": "execute" }
]
```

用户编辑 `dist/plugins/sandbox/plugin.json` 的 `consumes` 后，需要 **Rescan Plugins** 并重启 sandbox。Handler 集合和 Host 的流水线清单都来自同一份 manifest，不会漂移。

Sandbox 是可选件，`autostart` 默认 false，由用户在插件中心启动。

## 载荷

`tool.call` 载荷：

```json
{
  "name": "file_write",
  "arguments": { "path": "README.md", "content": "x" },
  "session_id": "s-1",
  "workspace": "E:/project",
  "call_id": "call-1"
}
```

Provider 接缝载荷（`file.write`、`shell.execute` 等）只有 `arguments` 与意图字段。`tool.call` 的工具名会通过 `tool.list` 映射到目标 `capability.method`。

## 裁决动作

| 动作 | 行为 |
|---|---|
| `allow` | 原样 `Proceed` |
| `confirm` | 暂停调用，等待用户允许/拒绝 |
| `modify` | 按 `modified_arguments` 覆盖参数后 `Proceed` |
| `deny` | 短路返回 Tool Result 包络，`is_error=true` |

规则匹配条件：`tool`、`capability`、`method`、`path_glob`、`command_pattern`，多个条件为合取。多个规则命中时按动作强度（deny > confirm > modify > allow）、priority、条件具体度、rule id 排序。没有规则命中时使用 `default_action`，默认 `allow`。

`modify` 规则必须带 JSON 对象形式的 `modified_arguments`；缺失时按 deny 处理。

## 确认流

`confirm` 会创建一次性 token，把待确认项推送到 `sandbox-confirm` 浮层，并阻塞当前 seam handler。SDK 的接收循环会继续服务 `sandbox.pending.list` 和 `sandbox.decide`。

- 允许：原调用继续进下一跳。
- 拒绝：返回 `sandbox_denied`。
- 超时：返回 `sandbox_confirm_expired`。
- turn 取消：清理 token，返回 `call_cancelled`。

确认面板提供「允许一次 / 拒绝」。它通过 `sandbox.pending.list` 恢复状态，通过 `sandbox.decide` 提交决定。

## 嵌套接缝去重

`tools` 分发 `tool.call` 后会用同一个 `call_id` 调用目标 provider，那次调用会再次进链。外层 `tool.call` 放行或确认后，sandbox 记录 `(call_id, capability, method)`，匹配的内层接缝直接放行，避免重复确认和重复审计。

如果希望按 provider 接缝逐层检查，把 `tool.call` 从 `consumes` 中移除即可。

## 审计

每次裁决向 Session 写入 `sandbox_decision` fact，meta 包含 `call_id`、工具、目标能力/方法、动作、规则、原因、参数摘要和 token。Session 不可用时回退到进程内有界环形缓冲。

## Settings

`config.json`：

```json
{
  "default_action": "allow",
  "confirm_ttl_seconds": 300,
  "rules": [
    {
      "rule_id": "r-1",
      "action": "confirm",
      "scope": { "capability": "shell", "method": "execute" },
      "priority": 100,
      "note": "命令执行前确认"
    }
  ]
}
```

设置页以列表编辑规则；规则行动作留空时按 `confirm` 处理。

## 已知边界

浏览器直接发起的调用不经过消费链，因此 sandbox 只覆盖插件发起的调用。当前 UI 只通过 `loop.turn` 驱动工具调用，工具调用本身由 agent 发起，仍在链上。
