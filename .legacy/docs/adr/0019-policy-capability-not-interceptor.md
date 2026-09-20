# Permission 走 Policy Capability，不恢复 Interceptor

工具执行前的 allow/ask/deny 由 provides `policy` 的 sandbox 插件实现；Agent Loop 在 callTool 前主动 `policy.decide`，ask 经 Host `agent.request` 回到 Render Medium。Host 星型路由只保留 closed 拒绝（ADR-0014）。

不把策略做成路由上的隐式中间件：旧 Waterfall/Interceptor 已删，恢复会使「谁否决了调用」藏在链路里且与默认 Loop 两张皮。代价是换 Loop 的插件必须自己接 policy，未接则等价全放行——用文档与金路径测试约束默认 agent 插件。
