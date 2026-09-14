# 移除 Waterfall / Interceptor

星型路由只保留 Host closed 拒绝；外部 Interceptor Capability、双层 Waterfall、审计链与 `-audit` 一并删除。横冲直撞阶段不需要策略拦截面；默认 Loop 本就未经 Waterfall。代价：插件星调不再有运行时改写/短路平面——若未来 sandbox 需要，应另立执行安全模型，而不是恢复旧 Interceptor 语义。

与「一切皆插件」不冲突：策略应做成 Capability 消费者或 Loop 替换，而不是挂在路由上的隐式中间件。
