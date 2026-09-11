# 02 — 流式回调与 presentation 信号

**What to build:** Host `CallStream` 支持边收边回调；接线 `presentation.stream` / `presentation.status`。

**Blocked by:** 01 — llm-openai

**Status:** resolved

- [x] `CallStreamOn`：带 id 的 evt 在 res 前回调
- [x] 识别 `presentation.stream` op=start|chunk|end 与既有 `llm.chunk`
- [x] Loop turn 边界 `OnStatus` running/idle
- [x] stream/status 不写入 Session Log
- [x] Card 路径不回归

## Answer

`wait.onEvent` + `collectEvent` 回调；`extractStreamDelta`；`Server.OnStreamDelta` / `OnStatus` 供 Render Medium。

## Comments

- 回调在 read-loop goroutine，须快速返回。
