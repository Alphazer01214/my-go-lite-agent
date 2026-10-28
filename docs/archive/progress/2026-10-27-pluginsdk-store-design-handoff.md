# pluginsdk 共享信息 map（Store）设计共识（2026-10-27）

> 状态：待实现（设计已逐点确认，代码尚未落地）
> 关联：[pluginsdk.md](../../platform/pluginsdk.md)、[CONTEXT.md](../../../CONTEXT.md)、[pluginsdk/context.go](../../../pluginsdk/context.go)

## 0. 背景与目标

诉求：插件可以通过 pluginsdk 直接访问一个共享信息的 map。动机是避免长信息多次传递导致效率降低。讨论中进一步明确：希望以实现 `context.Context` interface 的方式，让插件在 handler 里经 ctx 直接访问内存。

## 1. 架构约束（三条硬事实）

| 约束 | 出处 | 对设计的影响 |
|---|---|---|
| 插件是进程外可执行单元，不得直连其他插件 | CONTEXT.md、ARCHITECTURE.md | 跨插件共享的数据必须走 Frame 序列化，没有绕开进程边界的方案 |
| `context.Context` 过不了 stdio 管道 | Go 语义 | ctx 注入只能发生在各进程内部；跨进程共享与 ctx 体验是两层问题 |
| `Value` 契约：快速、无阻塞、无错误返回 | context 包约定 | Value 里不能藏 RPC；过线访问必须走显式方法（带 ctx 与 error） |

因此「直接访问内存」只对本进程内数据成立。设计问题从「能否直接访问内存」转为：数据何时过线、过几次、一致性谁保证。

## 2. 方案空间与取舍

| 策略 | 过线时机 | 跨插件 | 一致性 | 结论 |
|---|---|---|---|---|
| A. 进程内 map，ctx 注入 | 从不过线 | ✗ | 进程内一致 | 第一步采纳 |
| B. Host 权威，访问即过线 | 每次读写 | ✓ | 强一致 | 每次读过线，违背动机；留作第二步基线 |
| C. Host 权威 + SDK 缓存 | 首次读 + 写穿透 | ✓ | 有缓存失效问题 | 第二步候选，等真实用例 |
| D. 每条 Frame 附带快照 | 每条 req | ✓ | 快照一致 | 否决：长信息反复过线，与动机相反 |

效率问题的既有正解是传引用：model-context 的 `ModelContextID` 快照模式。共享 map 定位为小的、高频读的、多插件都要的环境信息（workspace、session 元数据、facts 索引）。长信息继续 owner 存储 + 传引用。两者互补。

附带发现：`plugins/agent/types.go` 与 `plugins/session/types.go` 草稿里的 `SessionInfo.Messages []ChatMessage` 若整份随 Call 传递，正是要避免的多次传递，应改为传 session 引用。此问题不属本设计，另行处理。

## 3. 定稿决策（逐点确认）

1. **数据策略**：两步走。第一步纯 SDK 进程内 map（策略 A），不碰协议与 Host。第二步（策略 C）等第一步跑出真实跨插件用例后单独设计。
2. **访问方式**：SDK dispatch 构造 handler ctx 时包一层私有类型。其 `Value` 对共享 key 返回本请求桶的 `*Store`，其余 key 委托父级。出站 Call 不传播。
3. **提取出口**：`pluginsdk.FromContext(ctx) (*Store, bool)` 是唯一出口。包装类型与 key 不导出，调用方不写裸断言。
4. **作用域**：与 Host Execution Context 解耦。仅拿 frame 的 `context_id` 作命名空间分桶，空串归全局桶。
5. **桶上限与生命周期**：LRU 上限 128 桶，防 `ctx-N` 递增导致无限增长。桶生命周期 = 进程生命周期。v1 无关闭通知，不清理，文档标注。
6. **签名**：方法带 ctx + error，第一天就有。进程内实现 error 恒为 nil，它是第二步零改动升级的签名位。
7. **值语义**：`any` 原样存，进程内不序列化。文档写前向兼容约定：入 Store 的值将来要能过线（JSON 可序列化）。`Set(nil)` 存 null。`Get` 用第二返回值区分「不存在」与「值为 nil」。
8. **命名**：类型 `pluginsdk.Store`，提取函数 `FromContext`。平台概念第一步叫「进程内共享存储」，第二步升格 Context Store。

## 4. 第一步 API 定稿

```go
func FromContext(ctx context.Context) (*Store, bool)

func (s *Store) Get(ctx context.Context, key string) (any, bool, error)
func (s *Store) Set(ctx context.Context, key string, value any) error
func (s *Store) Delete(ctx context.Context, key string) error
func (s *Store) List(ctx context.Context, prefix string) (map[string]any, error)
```

handler 用法：

```go
if store, ok := pluginsdk.FromContext(ctx); ok {
    _ = store.Set(ctx, "workspace", "/repos/demo")
    if v, found, _ := store.Get(ctx, "workspace"); found {
        // ...
    }
}
```

注入点：`pluginsdk/plugin.go` 的 dispatch 构造 handler ctx 处。当前是 `context.WithCancel(context.Background())`，外面包一层即可，取消语义由嵌入保留。

## 5. 第一步明确不做

跨插件共享、持久化、watch 与变更通知、Host 与 protocol 的任何改动。`internal/host/context.go` 空壳保留给第二步，第一步不动。

## 6. 已否决的备选

| 备选 | 否决原因 |
|---|---|
| 作用域含 Host 全局 map | 需要独立命名空间与清理策略，超出当前目标 |
| per-plugin 命名空间 | 进程内插件自有字段即可覆盖，平台层无增益 |
| 子上下文继承（读继承写隔离、快照拷贝） | 与 Host Execution Context 解耦后无父链可依赖；需要时再议 |
| 扁平方法 `p.StoreGet(ctx, contextID, key)` 一族 | 封装深度低；ctx 注入的调用点更短 |
| 两套签名并存 | 调用面翻倍 |
| 命名 `Context` / `SharedMap` / `ContextStore` | 分别与 Go `context.Context`、实际承诺、Host 术语冲突 |
| 一步到位 B 或 C | C 的缓存失效是全套最复杂部分，无用例时不应背上；B 每次读过线 |

## 7. 实施清单

| 项 | 位置 |
|---|---|
| Store、LRU 桶、包装类型、FromContext | `pluginsdk/context.go`（替换空壳） |
| dispatch 注入 | `pluginsdk/plugin.go` dispatch 一处 |
| 术语「进程内共享存储」 | CONTEXT.md |
| SDK 面一节（注入时机、API、桶、生命周期、前向兼容约定、error 恒 nil 说明） | docs/platform/pluginsdk.md |

实施完成后，本记录状态改为当前实现。
