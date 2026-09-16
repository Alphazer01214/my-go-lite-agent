# 02 — 清单与 Discovery

**What to build:** 用户把插件目录交给 Host，Host 扫描 `plugin.json` 并列出「机器上有哪些插件」（name、version、protocol、provides、consumes、entry）。坏清单给出可诊断错误。Discovery 只负责看见，不挂载。

**Blocked by:** 01 — Frame 回环：Host + echo 插件

**Status:** resolved

- [x] 扫描插件目录下的清单，产出 Discovery 结果
- [x] 清单至少校验 name/version/protocol/provides/entry；缺失或非法时失败信息可读
- [x] 能以 CLI 或明确输出列出已发现插件及其声明的 Capability
- [x] 主缝测试：给定含合法与非法清单的目录，断言发现集合与错误
- [x] 此阶段不因 Discovery 而拉起插件进程（与 Assembly 分离）

## Answer

`plugin.Manifest` + `discovery.Scan` 一层目录扫描；Host 增加 `-discover <dir>`，列出合法插件、stderr 报非法清单，有错误则 exit 1。集成测试覆盖合法列表与非法清单诊断；Discovery 路径不拉起进程。

## Comments

- 清单校验含 protocol==1、entry 文件存在；目录无 plugin.json 时静默跳过。
