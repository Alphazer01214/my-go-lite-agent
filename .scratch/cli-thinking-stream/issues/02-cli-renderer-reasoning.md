# 02 — CLI renderer reasoning 分离 + Windows VT

**What to build:** turnRenderer 区分 content/reasoning；Step 边界清 streamBuf；Windows 开 VT。

**Status:** ready-for-agent

- [x] onStream(delta, channel)
- [x] reasoning 不进 live answer 行
- [x] onTool reset streamBuf
- [x] vt_windows.go
