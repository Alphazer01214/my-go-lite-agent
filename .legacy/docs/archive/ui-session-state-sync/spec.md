# 进入 Session 后 UI 状态及时同步

Status: ready-for-agent

## Problem

点进一个已使用完全体（coding / tool_calling）的 Session 后：

- **Status Bar**（`agent-status`）只每 15s 轮询全局 `defaultScheme`，不监听 session 切换 / scheme 变更 / ensurePlugins。
- **agent-mode-panel** 仅 connectedCallback 时 refresh 一次。
- **Plugins 图** 预取于 boot，打开时才后台 refetch；session 进入后 mounted 集已变却不重绘。
- **Settings** 打开时才 load，不反映当前 session 实际生效的 scheme。

真源：scheme 写在 Session Log `turn_start.meta.scheme`；插件 mounted 真源是 Host `ensurePlugins` 结果。

## Solution

1. scheme / session 切换后 emit `__scheme` / `__session`，相关组件立即 refresh。
2. agent-status / agent-mode-panel 订阅上述事件，缩短或去掉纯轮询依赖。
3. plugins-panel 在 session select 与 scheme change 后强制 refetch。
4. 进入 session 时优先展示该 session 最近 `turn_start` 的 scheme（回退 defaultScheme）。

## Issues

- 01-emit-scheme-session-events
- 02-status-bar-live-refresh
- 03-plugins-graph-refetch
- 04-session-scheme-display
