# Session 作插件，Host 强制「模型可见即已记录」

**Status: superseded by ADR-0030（不变量实现位置）** — 目标「模型可见即已记录」不变；校验不再在 Host 星型中心，改由 session 插件 `validate` / Loop 自调（见 ADR-0030 下放表）。

Session Log 由独立插件提供 append/query/derive；Host 在 `agent/request` 前强制校验：进入 LLM 的内容必须能从 Session 重建。

与「一切皆插件」一致，Session 可替换（内存 / 文件 / 远端）。不变量放在星型中心（Host），而不是默认 Loop 内，这样外置 Loop 也不会削弱日志真源。代价是每步投影可能多一次 IPC；需要时可在 Host 做只读缓存，但缓存不得成为第二真源。
