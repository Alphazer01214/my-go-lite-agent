# CLI 流式 Thinking 重复打印

Status: ready-for-agent

## Problem

CLI `turnRenderer`：

1. `OnStreamDelta(delta)` **丢掉 channel**，reasoning 与 content 全部进同一个 `streamBuf`。
2. streamBuf **跨 Step 不清空**，多 Step 时 thinking 反复出现在 live 预览尾部。
3. `\r\x1b[2K` 依赖 ANSI；Windows 未开 VT 时清行失败 → 每次 delta 追加一行，表现为 thinking 重复刷屏。
4. settle 后再打全文，若 live 行未清掉则双重输出。

## Solution

1. `OnStreamDelta(delta, channel)`；CLI 只把 content 进 live 预览，reasoning 单独 dim 短预览或不进。
2. 每个 Step/工具边界 reset streamBuf。
3. Windows 启动时 `ENABLE_VIRTUAL_TERMINAL_PROCESSING`。
4. settle 清行后再打 markdown。

## Issues

- 01-stream-delta-channel
- 02-cli-renderer-reasoning
- 03-windows-vt-mode
- 04-step-buffer-reset
