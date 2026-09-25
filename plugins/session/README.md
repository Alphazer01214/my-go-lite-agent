# session

Append-only Session Log（真源）+ derive 投影。一切写入听外部；session 不自压缩。

## 提供的能力

| capability.method | 说明 |
|-------------------|------|
| `session.create` | 显式创建（幂等）；仅 agent 触发 |
| `session.append` | 唯一写入口；未知 id → `session_not_found` |
| `session.query` | 读 facts（`after_seq` / `limit` / `types`） |
| `session.derive` | Model Context 投影（chat `messages[]`） |
| `session.list` | 会话列表 |
| `session.info` | 元数据与计数 |

## 不变量

- 只追加；无 update / delete
- `seq` / `ts` 由 session 分配
- 损坏尾行 skip + 告警（软失败）

## 配置

`config.json`：`fullToolResults`（默认 8；`0` = 全部 tool 结果 stub）。  
环境变量 `SESSION_DATA_DIR`：数据目录（缺省 `./sessions`）。

## 命令

`/session list` · `derive` · `dump-trace` · `info`
