# Plugin Host Runtime

Status: ready-for-agent

## Problem Statement

用户需要一个**分发后即可扩展**的轻量 Go agent：拿到二进制与插件目录就能跑、能换能力，不要求阅读者编译源码，也不引入第三方运行时框架。现有 Go 生态常见路径都不可用或不可接受：进程内 `.so` 插件跨平台差（尤其 Windows）；编译期注册要求改代码重编译；完整 DI/插件框架又违背「核心零第三方」。用户还要求插件组织方式可被深入讨论与演进，而不是一次性拍死的工具注册表。

## Solution

建立 **Host 薄内核 + 进程外插件** 的运行时：Host 负责 Discovery、Assembly、生命周期、Frame 路由与双层 Waterfall；每个 Plugin 是「配置清单 + 可执行 + 静态文件」的独立进程，经 stdin/stdout 上的长度前缀 JSON Frame 与 Host 通信。跨插件调用一律星型经 Host。Session Log 由插件提供且是历史唯一真源；Host 强制「模型可见即已记录」。默认 Agent Loop 编译在 Host 内，但允许由外部插件替换同一 Capability。术语以仓库 `CONTEXT.md` 为准，架构约束以 `docs/adr/0001–0003` 为准。

## User Stories

1. 作为 agent 使用者，我想在不编译任何 Go 源码的情况下启动 Host 并挂载插件，以便拿到发行包即可运行。
2. 作为 agent 使用者，我想把插件目录指给 Host，由 Discovery 扫描出机器上可用的插件，以便不必手写每条绝对路径。
3. 作为 agent 使用者，我想用一份 Assembly 配置决定「挂载哪些、何序、何参数」，以便同一套插件二进制可组合出不同产品形态。
4. 作为 agent 使用者，我想在配置里显式点名某些插件、同时允许未点名插件仅被发现不被挂载，以便默认安全、按需启用。
5. 作为 agent 使用者，我想在 Assembly 结束时若 `consumes` 依赖未满足就 fail-loud 退出并看到缺失 Capability，以便启动问题立刻暴露。
6. 作为 agent 使用者，我想 Host 按依赖拓扑拉起插件进程，以便插件之间不必约定启动顺序。
7. 作为 agent 使用者，我想每次 Capability 调用有默认超时且可被清单覆盖，以便慢插件不会卡死整个 agent。
8. 作为 agent 使用者，我想插件进程崩溃时其全部 Capability 立刻变为 unhealthy 并让进行中调用失败，以便错误不被静默吞掉。
9. 作为 agent 使用者，我想崩溃后的插件在下一次被调用时 on-demand 重拉，以便服务可恢复且不会陷入无限重启风暴。
10. 作为 agent 使用者，我想 Host 退出时先关 stdin、等待 grace period 再强制结束插件进程，以便不留孤儿进程。
11. 作为插件作者，我想只实现 Function（收 Call Payload → 处理 → 返回 result / additionalContexts），以便简单能力不必关心 UI。
12. 作为插件作者，我想在同一插件上附带 Presentation 面，用纯函数从 args/result 投影 Presentation Card，以便 UI 能呈现问卷、diff 等结构化卡片且回放可重现。
13. 作为插件作者，我想把重量级 UI 拆成独立 Presentation 插件，以便 Function 与前端可独立演进。
14. 作为插件作者，我想通过 Host 请求其他 Capability，而不是直连其他插件进程，以便策略与审计不会被旁路。
15. 作为插件作者，我想 Function 一次调用返回结果后由 Loop 决定下一步，以便我不必在插件内编排 agent 循环。
16. 作为插件作者，我想在返回结果时附带 Additional Contexts，由 Loop 在工具结果之后写入 Session Log，以便我提供模型可见补充信息而不改写历史。
17. 作为插件作者，我想在需要异步通知模型时使用经 Host 的 `agent.inject`，以便消息落入下一次获准的 Model Context 而不误唤醒 agent。
18. 作为插件作者，我想清单声明 `provides` / `consumes` / `protocol` / `timeoutMs`，以便 Host 做就绪判断与协议协商。
19. 作为插件作者，我想静态文件默认仅本插件可读、显式声明后才导出，以便减少跨插件耦合。
20. 作为插件作者，我想用版本化 Frame（`v` 字段）与 Host 协商，以便协议演进时不静默错帧。
21. 作为策略作者，我想把审批、改写等领域策略做成 Interceptor 插件挂在 Waterfall 上，以便不改 Host 就能改行为。
22. 作为策略作者，我想 Host 内建强制 Waterfall 链（至少审计与取消），以便外部 Interceptor 全挂也不会丢失最低安全平面。
23. 作为策略作者，我想 Interceptor 能放行、改写或短路一次调用，以便实现权限拒绝与请求改写。
24. 作为会话维护者，我想 Session Log 是仅追加的事实流，历史只派生不单存，以便回放与恢复有唯一真源。
25. 作为会话维护者，我想 Host 在 `agent/request` 前强制校验 Model Context 可从 Session Log 重建，以便不变量不依赖某个具体 Loop 实现。
26. 作为会话维护者，我想 Session 以插件形态提供 append / query / derive，以便可替换存储实现（内存、文件等）。
27. 作为 agent 使用者，我想默认 Loop 在 Host 内开箱即用，注入 session / llm / tools / system-prompt 等 Capability，以便最小 Assembly 即可跑通一轮对话。
28. 作为高级用户，我想用外部插件替换默认 Loop（同一 Capability 名），以便试验不同循环策略而不改 Host 二进制。
29. 作为 LLM 适配器作者，我想 LLM 插件无会话状态，只消费传入的 messages，以便压缩与投影完全由上游负责。
30. 作为工具作者，我想注册面向模型的工具 schema 并由 Loop 分派，以便模型能调用我的 Function。
31. 作为前端作者，我想作为普通消费插件订阅 Presentation 相关事件与 Session 事实，以便渲染 UI 而不嵌进 Host。
32. 作为发行者，我想插件目录结构稳定（清单、可执行、可选 static），以便打包与文档一致。
33. 作为排障者，我想看到未加载/未激活条目的明确失败信息，以便 Assembly 错误可诊断。
34. 作为排障者，我想 Host 可转储实际 Assembly 树（挂载了谁、何参数），以便对照配置排查。
35. 作为平台维护者，我想核心逻辑仅依赖 Go 标准库（解析类第三方除外），以便审计与交叉编译简单。
36. 作为 Windows 用户，我想整条进程模型在 Windows 上一等公民（stdin/stdout、进程结束语义），以便不把 Linux 当唯一目标。
37. 作为学习者，我想规范名词写在 CONTEXT.md、难逆决策写在 ADR，以便后续实现不漂移。
38. 作为二次开发者，我想只依赖仓库提供的 Plugin SDK（Serve/Handle/Call/Emit）与公开 Frame 契约实现插件，而不必手写 stdio 解析与 Host 协商，以便把精力放在 Capability 逻辑上。

## Implementation Decisions

- **运行时形态**：进程外插件 + 星型经 Host（ADR-0001）。插件不直连；策略平面唯一。
- **依赖边界**：核心逻辑仅标准库；配置/清单解析允许第三方解析库。
- **Host 职责**：Discovery、Assembly、进程监督、Frame 编解码与路由、双层 Waterfall、生命周期、Session 不变量强制、默认 Loop。
- **Discovery vs Assembly**：扫描只负责「看见」；挂载由 Assembly 配置决定；支持目录扫描与显式点名并存。
- **清单契约**：至少含 name、version、protocol、provides、consumes、entry、可选 timeoutMs；静态资源默认私有、显式导出。
- **传输**：stdin/stdout，`uint32` 大端长度前缀 + JSON body；管道断开即视为插件死亡。
- **Frame 形状**（自研极简，非 JSON-RPC）：

```json
{ "v": 1, "id": "…", "type": "req|res|evt", "cap": "llm", "method": "stream", "payload": {}, "error": null }
```

  - `req`/`res` 以 `id` 对齐；`evt` 可无 id（广播）或带订阅 id。
  - 超时由 Host 计时；清单可声明 `timeoutMs`。
- **生命周期**：Assembly 启动期按拓扑拉起，依赖超时未满足 → fail-loud；运行期崩溃 → 全部 Capability unhealthy；on-demand 重拉；Host 退出 → 关 stdin → grace → 强杀。
- **Capability 就绪**：启动 fail-fast + 运行期 unhealthy/重拉（混合策略）。
- **双面模型**：Function 必选；Presentation 可选同体或独立插件（ADR 相关决策见 CONTEXT）。Presentation Card 为纯函数投影，禁止 I/O。
- **Function 契约**：一次调用返回 `result` 和/或 `additionalContexts`；不编排下一步；异步模型可见通知走 `agent.inject`。
- **禁止插件直接改写历史**：一切模型可见写入经 Session append 或 Loop 落盘路径。
- **Session**：插件形态提供 append/query/derive；Host 强制「模型可见即已记录」（ADR-0002）。
- **默认 Loop**：在 Host 内，可被外部插件替换同名 Capability（ADR-0003）。
- **LLM**：无状态适配器，只认 Call Payload 中的 messages；Memory/Session 不进入 LLM 插件内部状态。
- **Waterfall**：Host 内建强制链（审计、取消等最低集）+ 可选外部 Interceptor（放行/改写/短路）。
- **明确不移植**：HMR 热替换已编译代码、`!!js` 配置表达式、TS 类型合并、Proxy 隐式查找、兄弟分支可见、多层 isolate realm（v1）。
- **配置插值**：仅允许少量 Host 环境变量形式（如 `${ENV}`），不做通用表达式语言。

## Testing Decisions

- **唯一主缝：Host 进程边界。** 以真实 Host 可执行 + fixture 插件进程做集成测试；测外部可观察行为，不测 Host 内部包结构。
- **好测试的标准**：给定插件目录与 Assembly，启动 Host，驱动一轮输入，断言 Session Log 可派生内容、Frame 时序（可用 fixture 插件自记日志）、进程生死、退出码与错误信息。不断言内部函数调用顺序。
- **覆盖模块（经主缝）**：Discovery/Assembly、Frame 路由、生命周期与恢复、默认 Loop 一轮、Session 不变量拒绝路径、Waterfall 强制链与 Interceptor 短路。
- **Fixture 插件**：仓库内示例插件（如 echo、fake-llm、memory-session）作为合法叶节点，禁止用 mock 替换 Host 内部协作对象。
- **平台**：测试需在 Windows 上可跑；进程结束与管道关闭行为按 Windows 语义验证。
- **无既有 prior art**：仓库当前无测试；新测试体系与本缝一并建立。

## Out of Scope

- 真实 LLM 提供商网络调用与流式协议细节（用 fake-llm fixture）。
- 图形 UI / Web UI 实现（仅定义 Presentation 事件形状与消费方式）。
- HMR、脚本化插件运行时、进程内 `.so` 插件。
- 多 agent scope/isolate、subagent 编排、Goal/Ralph 等高级循环策略。
- 分布式 Host、远程插件（仅本机 stdio）。
- 配置通用表达式语言与复杂模板。
- 性能优化（IPC 批处理、零拷贝）——正确性优先。

## Further Notes

- 规范名词一律以 `CONTEXT.md` 为准；实现若与 ADR-0001/0002/0003 冲突，先改 ADR 再改代码。
- 参考实现思路来自 deepseek-harness / Cordis（无特权内核、可逆注册、inject 驱动、waterfall、事件溯源会话、seam 三角色），但映射到 Go 进程边界模型，不引入其 TS 运行时机制。
- 建议实现顺序：protocol/Frame → host process+router → discovery/assembly → **plugin SDK Serve** → 星型路由与 SDK Call → 生命周期 → session → 默认 loop → interceptor → presentation。
- 分工：Host `serve` = 进程监督 + Capability 路由；`pluginsdk` = 插件作者 API（Serve/Handle/Call/Emit）。原生插件与第三方同一路径。
- 仓库内正式插件（session/tool/llm/interceptor）一律基于 `pluginsdk`，保证作者路径与产品路径同一套 API。
- 后续 tickets 拆到 `.scratch/plugin-host-runtime/issues/`。
