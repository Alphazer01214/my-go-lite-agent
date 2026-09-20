# Shell 五块通用区域：左中右 + 上下；session 占左、中

**Status: accepted**

## Context

ADR-0011/0012 曾把主界面拆成具名槽位（`chat`、`trace`、`sidebar`、`statusbar`、`toolbar-right`），并在基座 `layout.json` / Shell chrome 里写死 Chat 与 Session trace 栏。用户裁决：Host/Shell **不得**对 session（或任何领域插件）特化；chat 与 trace 都必须完全由 session 插件提供。

初版理解「仅上下左右四块」有误；用户澄清：水平方向是 **左 / 中 / 右**，垂直方向仍有 **上 / 下**；**session 占用左与中**。

与 ADR-0012 基座示例（含 chat/trace slot）及 ADR-0024「statusbar 槽位」命名冲突：本 ADR **supersedes** 上述「基座具名领域槽位」部分；Status Bar 的「插件 status 组件挂底栏」模式保留，槽位 id 为通用 `bottom`。

## Decision

1. **基座 Layout 五槽**（region 与 id 同名，角色一律通用 `panel`）：
   - `top` · `bottom` · `left` · `center` · `right`
   - **不**在基座声明 session-view / session-trace / session-rail 等 preferred 组件。
2. **Shell chrome = 五块区域 + 框架自有 overlay**
   - `#region-top|bottom|left|center|right` 为插件挂载宿主；区域几何通用（flex/grid 填充），**无** per-plugin 选择器。
   - top 内可放置 Shell **L0 框架控件**（Plugins/Settings/导航/Host 诊断 `#status`），不得出现领域面板。
   - Settings / Plugin Graph 使用 Shell 自有 `#main-overlay`（非 Layout 槽、非 UISlots 词表）。
3. **区域占用**
   | 区域 | 占用方 | 内容 |
   |------|--------|------|
   | `top` | Shell | L0 chrome |
   | `left` | **session** | `session-rail` |
   | `center` | **session** | `session-workspace`（chat \| trace，组件内分栏） |
   | `right` | 其他插件 | 如 agent 面板；可空 |
   | `bottom` | 各插件 | `<plugin>-status` chips |
4. **UISlots 词表** = `top | bottom | left | center | right`（Shell 宿主槽）。
   - Manifest `ValidMountSlot` 仍允许加法贡献页的 slot id（slug 模式）。
   - PanelOp 仅接受 Shell 宿主槽（`ValidUISlot`）。
5. **`plugin.CurrentProtocol = 6`**：破坏 Manifest/UI 槽位契约；出厂插件 `protocol` 升 6 并改 mounts。
6. **领域语义只在插件**（ADR-0030 一脉）：Shell/`web/static` 不得因 chat/trace/session 做分支或专用几何。

## Considered Options

- 保留 `chat`/`trace` 槽名仅作 Layout role：仍使 Shell/词表携带领域名，否决。
- 仅四槽（无 `center`）且 session 整面挂 `left`：与用户澄清的「左中右」不符，否决。
- session 把 chat|trace 塞进 `right`：主工作区不应落在侧栏，否决。

## Consequences

- 破坏 UI 契约：旧 mounts（sidebar/chat/trace/statusbar/toolbar-right）须迁移；protocol 6 拒载未升级清单。
- session 负责 rail 与 workspace 内部几何/过滤条；Shell 不再画 Chat/Trace pane 头。
- 新 ADR/代码与本文件不一致时以本文件边界为准。
