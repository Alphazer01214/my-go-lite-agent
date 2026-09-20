# Host ↔ Medium 边界：统一逻辑，展示可替换

**Status: accepted（审批面实现被 ADR-0034 取代：choice 归 session 插件，Host/Medium 专用审批代码已删除；先应答者生效、超时即 deny 的竞态规则原样继承）**

ADR-0011 已定「双入口共享内核，分歧仅在前端 Medium」，但没枚举哪些算「展示手段」，边界因此被侵蚀：`OnToolApproval`（serve/serve.go:102）是**单槽 func 字段**，`internal/app/web.go:120` 在组合 `-serve -repl` 时把它从 Web 的 handler 覆盖成 CLI 的 prompt，于是 Web 发起的 turn 去读 CLI 的 stdin——stdin 非 TTY 时直接返回 deny，工具被静默拒绝。而 web.go:114-119 的注释还断言「Web-initiated turns still get the Web face via the same hook」，与实现不符。

**决策：逻辑在 Host 一份，展示后端可注册多个。**

- 决策与时序逻辑（`policy.ask` 的裁决序、等待窗口、超时即 deny、结果落 Session Log）归 Host，只有一份实现。
- 「怎么问用户」是展示后端：CLI 走终端 prompt，Web 走 SSE `tool_approval` + `/api/tool-approval`。**两者可同时注册，互不覆盖**。
- 同类横切面（agent 状态、render intent、流式 delta、panel op）一律照此办理。

判据：**凡是「单槽且赋值即替换」的 `On*` 钩子，迟早会让两侧逻辑分叉，应改为订阅列表或一个可注册的服务接口。** 现有 `wireRenderer`（internal/app/session_agent.go:184）用「保存-还原」四个钩子来规避覆盖，是同一个问题的临时绕法，随本 ADR 一并改造。

竞态规则（已评审定案）：审批请求**广播**给所有已注册后端，**先应答者生效**——先到的一方写入裁决并落 Session Log，其余后端立即收到「已决」通知并撤除各自的提示；无人应答时超时即 deny（现有语义不变）。不需要来源标记，也不设 Medium 优先级。展示类订阅（status / render / stream / panel）天然是广播语义，多个后端各自呈现，无竞态。

supersedes ADR-0011 中「分歧仅在前端 Medium」的模糊表述：分歧限于**展示实现**，不含决策与时序。
