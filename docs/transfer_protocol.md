# Transfer Protocol 插件数据传输协议

## 路径
host 监听所有来自插件的消息

## Frame

### 帧类型定义
消息帧，包括3类事件：evt, req, res
1. Request（req）— 要对方干活并等结果
对应场景：同步能力调用。

插件 → 插件：CallTo("session", "session", "derive", …)、agent 调 tools.call
插件 → Host 横切面：to=host 的 ensurePlugins / plugins / setPluginEnabled
Host/Medium → 插件：Web POST /api/call 最终变成 Host 发出的 req
约束（重构后的 L0 语义）：

必须点名 to（目标插件名）；Host 只按插件名转发，不按业务 cap 路由
cap + method 给接收方插件自己 dispatch，Host 不解读 payload
禁止 to == from（不能调自己）
代码依据：host/transport.go 的 routeRequest，pluginsdk/server.go 的 Call/CallTo。

2. Response（res）— 对应某次 request 的闭环
对应场景：一问一答的另一半。

与 req 用同一 id 对齐
成功：payload 为结果
失败：error = {code, message}（如 method_not_found、timeout、plugin_down）
Host 在插件间转发时会把内部 fwd-N 还原成调用方看到的原始 id
每个 req 最终应有且只有一个 res（超时/进程挂掉时由 Host 合成错误 res）。

代码依据：pluginsdk.Server.dispatch 处理入站 req 后写出 res；complete 按 id 消掉 pending。

3. Event（evt）— 不要应答的旁路
对应场景：过程信息 / 推送 / 流式。Host 不解释业务内容，只做中继或归因。

分两种细类：

A. 无 id 的广播 evt（Emit）

Host collectEvent 按 cap/method 分流到事件总线，再扇出给 Medium/SSE
典型：
presentation.*：card / render / panel / stream / status
泛化 TopicEvent（"evt"）：如 session 的 choice.ask 审批卡（ADR-0034）
B. 带 id 的归因 evt（EmitTo）

标注属于某次在途 req
Host 挂到该 pending 调用的 wait.events，不广播
典型：LLM 流式 delta——llm.complete 的 req 还没返回，但中间 token 以 EmitStreamTo(reqID, …) 推出；调用方 callStream 能边等边收

### ID
用于标识唯一的 frame，分两类：
1. OriginalID 表示 **插件发出的ID**，格式为 `pluginname-xxx`
2. ForwardID 表示 **Host转发的ID或目标插件回复的ID**，格式为 `fwd-xxx`

在 Host 中维护了 id-wait 的 pending map 实现回调处理 

### 消息传输过程
一次传输需要进行完整的检查。

#### req
- is host closed
- is cap == host?
- is target == from?
- is target plugin mounted? disabled? 

#### res


### 格式
- 4byte 消息长度
- JSON body
