# 03 — Assembly 挂载

**What to build:** 用户用 Assembly 配置指定挂载哪些插件、何参数；未点名的已发现插件不被拉起。Host 可转储实际 Assembly 树，便于对照配置排障。

**Blocked by:** 02 — 清单与 Discovery

**Status:** resolved

- [x] Assembly 输入决定挂载集合（显式点名；与 Discovery 结果相交）
- [x] 仅被挂载的插件才会被 Host 拉起
- [x] 可输出实际 Assembly 树（插件名、是否挂载、关键参数）
- [x] 引用不存在的插件时有明确警告或失败（与 fail-loud 策略一致）
- [x] 主缝测试：两种配置启动同一插件目录，断言拉起集合与 dump 内容不同

## Answer

`assembly.Load` + `assembly.Resolve`：配置 `{"plugins":["…"]}` 与 Discovery 相交；缺失名 fail-loud。Host：`-plugins <dir> -assembly <file> [-dump]`，仅对 Mounted 插件做一次 Frame 探测启动。集成测试覆盖 A 只挂 alpha、B 挂 alpha+beta、引用 ghost 失败。

## Comments

- 参数字段（config per plugin）留给后续票；本票只做名字点名挂载。
