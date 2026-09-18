# session

File-backed Session Log 插件：JSONL 追加事实流，提供会话与 Trace 的 Web Panel。

## 提供

- Capability `session`：append / query / derive / info / list / create / current…
  - `append` 接受可选 `ts`（UnixMilli）；`>0` 时沿用调用方时间戳（如 reasoning hop-end），否则由 session 盖章
- 命令：`/session dump-trace | list | derive | current | info`
- UI（ADR-0031：Shell 为 `top|bottom` + `left|center|right`；session 占 **左、中**）：
  - `session-rail` —— 挂 `left`：会话列表。**只按工作区路径分组**（ADR-0020）
  - `session-workspace` —— 挂 `center`：组件内 **chat | trace** 分栏；chat 与 trace 全部由 session 提供
  - `session-view` —— workspace 内 chat 栏（含新建会话首页）
  - `session-trace` —— workspace 内 trace 栏（类型过滤条在组件内部）
  - `session-status` —— 挂 `bottom`：工作区 / 当前会话 / 事实数 chip

## 配置

无独立 config.json；会话文件在插件目录下 `sessions/`。

## Manifest

- `autostart: true`（核四件之一）
- `protocol: 6`（Shell 五区域槽位契约）
