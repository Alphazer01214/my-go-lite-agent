# 01 — tool schema 增加 severity

**What to build:** filetools / shelltools / webtools / skill-manager 的 tools.list schema 声明 `severity`（low|medium|high），由插件作者设定。

**Status:** resolved

- [x] filetools: read/grep/glob=low, write/edit=medium
- [x] shelltools: shell=high
- [x] webtools: web_fetch/web_search=low
- [x] skill-manager: load_skill=low

## Answer

refactor-host-boundary Z1 对账确认：本票内容已在既有实现中落地（见上方勾选项
与对应 ADR/测试），状态由 ready-for-agent 滞后修正为 resolved，非本轮新开发。
