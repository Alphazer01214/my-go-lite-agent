# sandbox

`sandbox` 在工具调用链上做权限裁决。它只裁决，不执行工具。

## 状态

当前实现 `allow`、`modify`、`deny` 三态。确认流、确认面板与 Prompt 贡献尚未实现。目标契约见 [docs/design/plugins/08-sandbox.md](../../docs/design/plugins/08-sandbox.md)。

## 消费

| 键 | 说明 |
|---|---|
| `tools.call` | 主接缝。模型发起的一次工具调用。 |
| `file.write` | 直连文件写入。 |
| `file.replace` | 直连文件替换。 |
| `file.patch` | 直连文件补丁。 |
| `shell.execute` | 直连命令执行。 |

`priority` 是 500，所以它排在 `tools` 之前。工具分发对目标能力的调用会重新进链，因此直连调用也过裁决。

## 依赖

`tools` 是硬依赖。`session` 与 `tools` 是弱依赖，缺失时插件照常启动。

缺少 `session` 时审计退化为进程内的有界环形缓冲，不返回错误。

## 缺席语义

插件不在链上时这一跳被跳过，调用直接交给提供方。缺席即放行是部署约定，不是平台保证。

## 判定顺序

1. 命中规则。动作优先级是 `deny` 高于 `modify`，`modify` 高于 `allow`。同级比权重、范围与规则号。
2. 未命中规则。按风险刻度取档。
3. 风险档没有覆盖。按 `default_action` 判定。

## 风险刻度

| 范围 | 含义 | 默认动作 |
|---:|---|---|
| 0 到 49 | 只读或可逆的本地修改 | `allow` |
| 50 到 79 | 显著的本地修改 | `deny` |
| 80 到 100 | 任意执行或破坏 | `deny` |

工具查不到时按风险 70 判定。

目标映射是 50 到 79 与 80 到 100 都走 `confirm`。确认流还没有实现，所以这两档当前按 `deny` 执行。确认流落地后只改配置，不改代码。

## 配置

`config.json`：

```json
{
  "default_action": "allow",
  "risk_bands": [
    { "max_risk": 49, "action": "allow" },
    { "max_risk": 79, "action": "deny" },
    { "max_risk": 100, "action": "deny" }
  ],
  "rules_file": "rules.json",
  "audit_retention_days": 30
}
```

- 配置文件不存在、解析失败或字段非法时回落到默认值。
- `rules_file` 只能是插件目录下的文件名。
- 风险档按 `max_risk` 升序读取，第一档覆盖该风险值。
- `audit_retention_days` 只约束进程内环形缓冲。会话事实日志不由本插件裁剪。

## 规则

规则写在 `rules.json`，也可以用 `sandbox.set_rule` 写。规则集只升不降：

- 更宽的放行规则被拒绝。
- `deny` 规则不能改动作，也不能删除。
- 收窄范围或改成更严的动作都允许。

## 启动

本插件的 `autostart` 是 `false`。默认部署没有裁决链，插件缺席即放行。需要裁决时从插件中心启动它。

启动方式与缺席语义见 [docs/platform/manifest.md](../../docs/platform/manifest.md)。

方法契约见 [communication.md](communication.md)。
