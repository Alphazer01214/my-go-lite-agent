# 01 — filetools：read_file 默认 limit=500

**What to build:** `read_file` 未指定 limit 时默认 500 行；硬顶 5000；截断时提示 offset/limit（已有 remaining 提示则保留）。schema description 同步。

**Status:** resolved

- [x] 默认 limit 500，硬顶 5000
- [x] 截断提示含 offset
- [x] 主缝测试 `TestReadFileDefaultLimit`

## Answer

`defaultReadLines=500`；未传 limit 时用默认，显式 limit 仍可到 5000。

## Comments

- 与 derive stub 正交：工具层少进 log，stub 管已进 log 的历史。
