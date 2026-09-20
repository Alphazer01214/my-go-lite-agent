# 03 — host -repl 多轮状态机

**What to build:** `host.exe -repl`：stdin 循环 → `RunTurn`；同进程同 Session；实时流式；Ctrl+C/exit 退出。

**Blocked by:** 02 — stream 回调

**Status:** resolved

- [x] `-repl` 与 `-turn` 并存
- [x] 每轮 prompt + RunTurn + 实时 chunk
- [x] exit/quit/EOF 退出并 Close
- [x] 主缝：两轮输入同 Session

## Answer

`runREPL`：`OnStreamDelta` 实时打印；`TestREPLTwoTurnsSameSession`。

## Comments

- 状态机在 Host CLI 内（Q6=A）。
