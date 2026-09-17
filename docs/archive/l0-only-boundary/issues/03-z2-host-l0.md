# 03 — Z2 Host 清零

Status: ready-for-agent

翻绿 C13/C15：删 AgentRequest/RunTurn/tools 领域解析/能力别名；
agent.* → host.ask/tell；锁进 loop 插件。

依赖：02
阻塞：Z3/Z4
决策点：tools 扇出 — 通用 multiOwner vs 独立插件（倾向 multiOwner）
