# 01 — OnStreamDelta channel + CLI 渲染

**Answer:** Host hook is now `OnStreamDelta(delta, channel)`. CLI turnRenderer keeps reasoning out of the live answer line, resets streamBuf on each tool hop, and enables Windows VT so `\r\x1b[2K` actually clears.

**Status:** resolved
