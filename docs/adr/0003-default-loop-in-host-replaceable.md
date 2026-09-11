# 默认 Agent Loop 在 Host，Capability 可外置

默认 Loop 编译进 Host 进程（注入 session / llm / tools / system-prompt 等 Capability）；同一 Capability 名允许由外部插件进程提供，从而替换循环实现。

全外置 Loop 会让最小 demo 也要先拉起循环进程，冷启动与调试成本过高；Loop 死锁在 Host 则失去「loop 也是插件」的可替换性。B2 折中：可用性优先，替换权保留。
