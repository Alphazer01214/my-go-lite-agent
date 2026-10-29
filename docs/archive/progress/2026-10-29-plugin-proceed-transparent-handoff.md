# Plugin Proceed 透明转发设计（2026-10-29）

> 状态：待实现（设计已确认，代码尚未落地）
> 关联：[pluginsdk.md](../../platform/pluginsdk.md)、[protocol.md](../../platform/protocol.md)、[pluginsdk/plugin.go](../../../pluginsdk/plugin.go)、[internal/host/pipeline.go](../../../internal/host/pipeline.go)、[internal/host/transport.go](../../../internal/host/transport.go)

## 0. 背景与目标

诉求：多个插件可以消费同一个 `(capability, method)`。

Consumer 拦截请求并做处理，然后调用 `Proceed` 把同一个请求继续交给下一跳。

Consumer 不产生当前请求的业务 Response。

只有链路终点插件返回业务 Response。

Consumer 仍然可以调用 `p.Call` 发起自己的独立请求。

业务层面只保留一个 `req-res` 对。

## 1. 现状边界

| 现状 | 影响 |
|---|---|
| `pluginsdk.dispatch` 在 handler 返回后自动写一个 `res` | Consumer 转发后会多写一个业务 Response |
| `Request.Proceed` 返回下游业务结果 | Consumer 必须接收并转发终点 Response |
| `internal/host/pipeline.go` 等待下游结果后回复 `host.proceed` | 下游 Response 会先回到 Consumer |
| `wait` 已保存原始 `caller` 与 `frameID` | 可复用它们直达原始调用方 |

`host.proceed` 仍可有一个内部控制 ack。

这个 ack 不属于业务 `req-res` 对。

## 2. 定稿决策

1. `Proceed` 是当前请求的终端转发操作。
2. `Proceed` 成功后不返回下游业务结果。
3. `pluginsdk` 只用本地状态阻止自动写业务 `res`。
4. Host 不引入 `delegated` 概念，只看 Frame 与 pending 映射。
5. 下一跳 pending 保留原始 `caller` 与 `frameID`。
6. 终点 `res` 直接改回原始 Request ID 并发给原始调用方。
7. Consumer 自己的 `p.Call` 保持原有 `req-res` 语义。
8. 没有下一跳 Consumer 时，Host 回退到 Route Owner。

## 3. pluginsdk 改动

### 3.1 Request 状态

```go
type Request struct {
    // ...
    proceed     func(context.Context, json.RawMessage) error
    proceedOnce atomic.Bool
    proceeded   atomic.Bool
}
```

`proceeded` 只服务本进程 dispatch。

它不是 Host 协议字段。

### 3.2 Proceed 契约

```go
func (r *Request) Proceed(
    ctx context.Context,
    payload json.RawMessage,
) error
```

成功语义：当前请求已交给下一跳。

失败语义：转发未发生，Consumer 仍可返回错误 Response。

重复调用返回 `flow_already_proceeded`。

### 3.3 dispatch 规则

```go
out, err := handler(ctx, req)

if req.proceeded.Load() {
    return
}

if err != nil {
    res.ErrorCode = Code(err)
    res.ErrorMsg = err.Error()
} else {
    res.Payload = out
}

_ = p.writeFrame(res)
```

`Proceed` 成功后，`dispatch` 不写当前请求的业务 `res`。

`Proceed` 失败时，仍走原有 Response 路径。

### 3.4 Consumer 用法

```go
func handle(
    ctx context.Context,
    req *pluginsdk.Request,
) (json.RawMessage, error) {
    if _, err := p.Call(
        ctx,
        "audit",
        "record",
        payload,
    ); err != nil {
        return nil, err
    }

    return nil, req.Proceed(ctx, req.Payload)
}
```

## 4. host 改动

### 4.1 继续转发时保留原始目标

`hostProceed` 从当前 pending 读取：

```go
rootCaller := current.caller
rootFrameID := current.frameID
```

下一跳 `wait` 使用：

```go
nextWait := &wait{
    caller:  rootCaller,
    frameID: rootFrameID,
    // target、pipeline、contextID 等字段照常填写。
}
```

### 4.2 终点 Response 直达原调用方

现有 `handleResponse` 可继续使用：

```go
res.ID = wt.frameID
res.From = wt.targetPlugin
h.write(wt.caller, res)
```

终点 Response 的 ID 是原始 Request ID。

终点 Response 的目标是原始调用方。

### 4.3 host.proceed 返回内部控制 ack

Host 成功把请求交给下一跳后，只返回：

```json
{
  "accepted": true
}
```

Host 不把终点业务 Response 返回给 Consumer。

### 4.4 finishProceed 不等待业务结果

当前实现应删除对下游 `resCh` 的等待。

`finishProceed` 只完成：

1. 选择下一跳；
2. 创建 continuation pending；
3. 写出下一跳 `req`；
4. 给 Consumer 返回内部控制 ack。

终点 `res` 由 continuation pending 直接完成原始请求。

### 4.5 旧 pending 的处理

Consumer 成功 `Proceed` 后，旧的 Consumer pending 应删除。

Host 忽略 forwarded Request 的迟到业务 `res`。

必要时记录 diagnostic。

### 4.6 缺席回退

`finishProceed` 找不到下一个 Consumer 时，回退到 `(capability, method)` 的 Route Owner。

没有 Route Owner 时，错误返回原始调用方。

### 4.7 Event

下游归因 Event 默认直接发给原始调用方。

Event ID 使用原始 Request ID。

## 5. 不改动范围

以下内容不改动：

- `protocol.Frame` 基本结构；
- `host.register`；
- `consumes` 声明；
- `priority` 选择；
- `p.Call`；
- Consumer 自己的独立请求；
- 原始 Request ID 恢复逻辑；
- `internal/host/cancel.go` 的取消查找机制。

## 6. 验收测试

1. 两个 Consumer 加一个 Provider，终点 Response 回原始调用方。
2. Consumer 的 `p.Call` Response 返回 Consumer。
3. Consumer 不写当前请求的业务 `res`。
4. 重复 `Proceed` 返回 `flow_already_proceeded`。
5. Provider 错误直接回原始调用方。
6. `Proceed` 后取消传播到当前下游。
7. Consumer 转发后返回 Response，Host 不重复响应。
8. 下游 Event 关联原始 Request ID。
9. 无 Consumer 时自动回退 Provider。

## 7. 实施顺序

1. 修改 `pluginsdk/plugin.go` 的 `Request` 与 `dispatch`。
2. 修改 `internal/host/pipeline.go` 的 continuation pending。
3. 修改 `hostProceed` 与 `finishProceed` 的 ack 规则。
4. 修改 `internal/host/transport.go` 的迟到 Response 处理。
5. 补齐集成测试。
6. 实现完成后更新平台文档状态。
