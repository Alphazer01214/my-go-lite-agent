# sandbox

`sandbox` 是 Frame Pipeline Consumer，不执行文件、命令或网络操作。

## 检查点

它消费 `tools.call`，以及 File、Shell、Web 的高风险入口。直接调用 Provider 仍会经过检查。

## 默认策略

| 风险 | 行为 |
|---|---|
| low | 放行 |
| medium | 放行并记录 |
| high | 拒绝 |
| unknown | 拒绝 |

## 权限文件

默认读取 `../../config/permissions.json`，可用 `LITEAGENT_PERMISSIONS_FILE` 覆盖。

```json
{
  "defaults": {"low": "allow", "medium": "allow", "high": "deny"},
  "rules": [
    {
      "capability": "file",
      "method": "write",
      "action": "allow",
      "workspace_prefix": "E:/project"
    }
  ]
}
```

规则按数组顺序匹配。字段省略表示通配。`workspace_prefix` 必须匹配调用 Workspace。

完整规则见 [communication.md](communication.md)。
