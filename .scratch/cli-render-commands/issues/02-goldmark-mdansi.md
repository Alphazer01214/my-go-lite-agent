# 02 — goldmark 接入 mdansi

**Status:** resolved

## Answer

`github.com/yuin/goldmark` 解析 CommonMark AST，`render/mdansi` 负责 ANSI 输出（标题/列表/围栏/引用/链接）。保留 `Plain`/`Indent`。
