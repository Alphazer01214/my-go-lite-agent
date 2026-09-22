# LiteAgent Plugin Runtime

Host 薄内核 + 进程外插件。插件与 Host 之间、插件与插件之间只经 Host 转发，通过长度前缀 JSON Frame 通信。

## Language

**Host**:
插件宿主，负责发现并启动插件进程，并按 Capability（及 method）转发 Frame。Host 自身实现 Host 特有 Capability（一切皆插件）。
_Avoid_: server、runtime（指宿主时）

**from**:
Frame 来源插件名。由 Host 注入，插件不可伪造。**每条投递的 Call 都必须带上**，便于溯源、日志、依赖图；**不参与路由**。
_Avoid_: sender、source

**Plugin**:
进程外可执行单元，由 Host 启动，经 stdio 与 Host 交换 Frame。

**Frame**:
Host↔Plugin 的一条完整消息：4 字节大端无符号长度 + JSON body。
_Avoid_: message（指帧时）、packet

**Request**:
`type=req` 的 Frame，要求对端执行 `capability.method` 并返回结果。
_Avoid_: call frame

**Call**:
pluginsdk 发起一次同步 Request 并等待 Response；经 Host 按 `capability.method` 转发。
_Avoid_: send、invoke（作 API 名时）

**Response**:
`type=res` 的 Frame，与某个 Request 用同一 `id` 闭环。
_Avoid_: reply、result frame

**Event**:
`type=evt` 的 Frame，无需应答的旁路消息。
_Avoid_: notification、broadcast frame

**Capability**:
能力名。manifest 的 `provides` 声明属主；Frame 的 `capability` + `method` 是**路由与 dispatch 的唯一键**，Host 按此转发。
_Avoid_: cap、ability、face

**Requires**:
声明需要的能力名；弱依赖，缺失不阻断启动。
_Avoid_: consumes

**DependsOn**:
硬依赖的具体插件名（装配/依赖图用；不参与 Frame 路由）。
_Avoid_: depends、dependsOn 当能力名用

**Provides**:
插件对外声明的能力名列表（观测/依赖图；路由真源是已注册的 `(capability, method)`）。
_Avoid_: abilities

**Identity**:
插件进程身份 = 插件名，进程创建时声明一次。仅用于日志、依赖图、观测；**不用于路由**。与 handler 注册（Capability.method）分离。
_Avoid_: plugin name 在 Register 里重复声明、按名字寻址
