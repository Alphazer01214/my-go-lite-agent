# ARCHITECTURE

Host 薄内核 + 进程外插件。插件之间、插件与 Host 之间只经 Host 星型转发，线格式为长度前缀 JSON Frame。名词见 [CONTEXT.md](CONTEXT.md)，线契约见 [docs/protocol.md](docs/protocol.md)。

## 1. 总览

```text
        ┌────────── plugins/* ──────────┐
        │  session   llm   agent   …    │
        └──────┬────────┬────────┬──────┘
               │ stdio  │        │
               ▼        ▼        ▼
           ┌────────────────────────┐
           │      internal/host     │  路由 / pending / 挂载
           └────────────────────────┘
                     ▲
                     │ pluginsdk.Call / Emit
           ┌────────────────────────┐
           │       pluginsdk        │  插件唯一入口
           └────────────────────────┘
```

- 无插件直连、无第二套路由。
- Frame 只有 `req` / `res` / `evt` 三类。

## 2. 分层

| 层 | 路径 | 职责 |
|----|------|------|
| 线契约 | `protocol/` | Frame 编解码与错误码唯一定义 |
| 插件 API | `pluginsdk/` | Register / Call / Emit / Serve；pending 与 handler 分发 |
| Host | `internal/host/` | 能力路由、forward、pending 闭环、进程健康 |
| 插件 | `plugins/*` | 业务 handler；只 import `pluginsdk` |

## 3. 数据流与 pending

| 方向 | 行为 |
|------|------|
| `Call` → `req` | 登记 `pending[id] = chan`，等 `res` |
| 归因 `evt`（带 id） | 同 channel；`Call` 丢弃，`CallWithCallback` 进 callback |
| `res` | 投递后 **close** channel |
| 无 id `evt` | SDK 侧丢弃（v1 无订阅扇出） |
| 断连 | `fail()` 关闭剩余 channel，Call 返回 `connection closed` |
| 入站 `req` | `dispatch` → handler → 写回一条同 id `res` |

## 4. 路由与身份

- 路由键 = `(capability, method)`，全局唯一属主。
- 插件名 / `from` / id 前缀只做观测与溯源，**不参与路由**。
- 跨插件能力经 Host `register` 上报后查表转发；自调用合法（环回）。

## 5. 目录对应

| 契约 | 实现 |
|------|------|
| Frame / 错误码 | `protocol/` |
| 插件开发面 | `pluginsdk/` |
| Host 路由与转发 | `internal/host/` |
| Manifest / 发现 | `internal/plugin/` |
| 业务插件 | `plugins/`（包结构见 [docs/plugin-dev.md](docs/plugin-dev.md)） |

## 6. 非目标（v1）

- 无 shutdown / restart Frame（生命周期归进程与 Host）。
- 无按插件名寻址（无 `CallTo` / `EmitTo`）。
- 无 ctx / 取消 / 调用超时。
- 广播 `evt` 的跨插件扇出未实现。
