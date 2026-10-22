# sandbox

`sandbox` 在工具调用链上做权限裁决。它只裁决，不执行工具。

## 状态

当前实现 `allow`、`confirm`、`modify`、`deny` 四态。命中 `confirm` 时调用会暂停，右下角出现确认面板；
用户允许后同一次调用继续执行，拒绝或超时按失败返回。

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

`tools` 是硬依赖。`session` 是弱依赖，缺失时审计退化为进程内的有界环形缓冲，插件照常启动。

## 缺席语义

插件不在链上时这一跳被跳过，调用直接交给提供方。缺席即放行是部署约定，不是平台保证。

## 判定顺序

1. 命中规则。动作优先级是 `deny` 高于 `confirm`，`confirm` 高于 `modify`，`modify` 高于 `allow`。同级比权重、范围与规则号。
2. 未命中规则。按风险刻度取档。
3. 风险档没有覆盖。按 `default_action` 判定。

## 风险刻度

| 范围 | 含义 | 默认动作 |
|---:|---|---|
| 0 到 49 | 只读或可逆的本地修改 | `allow` |
| 50 到 79 | 显著的本地修改 | `confirm` |
| 80 到 100 | 任意执行或破坏 | `confirm` |

工具查不到时按风险 70 判定。

## 用户确认

确认是阻塞式的。裁决方不短路返回，而是在自己的 handler 里等待：

1. 命中 `confirm` 后创建一次性令牌，写一条 `sandbox_decision` 审计事实，并把待确认列表推给 `sandbox-confirm` 面板。
2. 面板显示工具、风险、理由、参数摘要、会话与剩余时间。
3. 用户选择“允许一次”或“拒绝”，面板调用 `sandbox.decide`。
4. 原调用在允许后继续进下一跳；拒绝返回 `sandbox_denied` 包络。
5. 超过 `confirm_ttl_seconds` 返回 `sandbox_confirm_expired` 包络。取消轮次会清掉令牌并写终态审计。

令牌绑定会话与调用摘要，只存在内存里，进程重启后全部失效。

`tools.call` 放行后，`tools` 会用同一个 `call_id` 再调一次直连能力（如 `file.write`）。
sandbox 用短时标记识别这次续接，不会再询问第二遍，也不会重复写审计。

## 配置

`config.json`：

```json
{
  "default_action": "allow",
  "risk_bands": [
    { "max_risk": 49, "action": "allow" },
    { "max_risk": 79, "action": "confirm" },
    { "max_risk": 100, "action": "confirm" }
  ],
  "rules_file": "rules.json",
  "confirm_ttl_seconds": 300,
  "audit_retention_days": 30
}
```

- 配置文件不存在、解析失败或字段非法时回落到默认值。
- `rules_file` 只能是插件目录下的文件名。
- 风险档按 `max_risk` 升序读取，第一档覆盖该风险值。
- `confirm_ttl_seconds` 的范围是 30 到 3600。
- `audit_retention_days` 只约束进程内环形缓冲。会话事实日志不由本插件裁剪。

## 规则

规则写在 `rules.json`，也可以通过 `sandbox.set_rule` 或插件中心 → 设置 → `sandbox` 的规则列表编辑。

- `sandbox.set_rule` 保持只升不降：更宽的放行规则被拒绝，`deny` 规则不能改动作或删除。
- 设置页是人直接编辑策略，可以自由增删改所有持久规则，包括删除 `deny` 规则。
- 设置页只显示和保存持久规则；会话级规则仍在内存里。

## 启动

本插件的 `autostart` 是 `true`。默认策略允许只读调用，写文件与命令执行会先请求确认。

方法契约见 [communication.md](communication.md)。
