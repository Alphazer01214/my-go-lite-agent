# Sandbox ask 确认链路（Web/CLI）

Status: ready-for-agent

## Problem

policy.ask 后两侧 Render Medium 都表现成「卡在工具调用」：

1. **Web**：Host 已 `broadcast(tool_approval)` 并等 `/api/tool-approval`，但 `web/static/app/events.js` 的 EventSource **没有订阅 `tool_approval` topic**，session-view 的 `LiteAgent.on('tool_approval')` 永远收不到 → `window.confirm` 不弹 → Host 2 分钟超时 deny。
2. **CLI**：`OnToolApproval` 已接 `cliToolApproval`（Scanln），但若 terminal 未开 VT 或 prompt 被流式行冲掉，用户看不见确认；且默认 `defaultAction=allow`、无 severity 时几乎不会走到 ask。

## Solution

- events.js 订阅 `tool_approval` 并 fan-out。
- sandbox 按 tool severity 默认 ask（见 sandbox-tool-severity）。
- 金路径测试：fixture policy.ask → confirm 往返。

## Issues

- 01-web-sse-tool-approval
- 02-cli-ask-visibility
- 03-ask-roundtrip-test
