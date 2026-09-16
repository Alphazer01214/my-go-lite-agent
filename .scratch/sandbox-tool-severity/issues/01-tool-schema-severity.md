# 01 — tool schema 增加 severity

**What to build:** filetools / shelltools / webtools / skill-manager 的 tools.list schema 声明 `severity`（low|medium|high），由插件作者设定。

**Status:** ready-for-agent

- [x] filetools: read/grep/glob=low, write/edit=medium
- [x] shelltools: shell=high
- [x] webtools: web_fetch/web_search=low
- [x] skill-manager: load_skill=low
