# 05–07 — Manifest/Command 平面 + llm-openai config

**Status:** resolved

## Answer

Manifest 增加 `description`/`commands[]`；name `[a-z0-9-]+`；与原生命令冲突整体拒载。REPL：`/help` `/lp` `/refresh` `/exit` + `/plugin subcmd args` → `cap=commands,method=call`。llm-openai 实现 `config` get/set 写 config.json。
