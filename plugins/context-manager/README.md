# context-manager

`context-manager` 当前是上下文压缩原型。它没有 `main()`、`plugin.json` 或已注册 Capability，不能作为进程外插件挂载。

## 1. 当前代码

源码位于 `plugins/context-manager/main.go`。当前只定义：

| 类型或函数 | 状态 |
|---|---|
| `History` | 原型结构体 |
| `chatMessage` | 上下文消息 |
| `toolCall` | 工具调用 |
| `tool` | 工具描述 |
| `compact()` | 当前原样返回输入 |

`History.messages` 是未导出字段，不能由包外代码填入。原型尚未形成稳定 Interface。

## 2. 目标职责

目标 `context-manager` 提供：

```text
memory.assemble
```

它读取 Session 或上下文摘要，返回适合 LLM 的消息列表。Agent 在 `memory.assemble` 缺失时回退到 `session.derive`。

## 3. 前端

第一阶段没有 Component 和 Settings。

## 4. 文档

方法和通信计划见 [communication.md](communication.md)。
