# 03 — UI-only 插件合法化

**What to build:** 纯视图插件成为合法形态：Manifest 带 `ui` 块时 `entry` 可省。Discovery 只见其声明；Assembly 可挂载；Host 不为其启动进程、不占 capability 注册表；Web Medium 经 /api/plugins 暴露其 UI Entry 并服务其资产；CLI Medium 无视之。校验规则改为「entry 与 ui 至少其一」。进程为 Frame 而存在——纯视图插件无 Frame 可说。

**Blocked by:** —

**Status:** resolved

- [x] 无 exe 的 fixture 插件通过 Discovery / Assembly 并被挂载
- [x] 浏览器渲染其 Panel Component；host 无对应子进程
- [x] 既有插件（exe+ui、仅 exe）不受影响；缺 entry 且缺 ui 仍被拒绝
- [x] CLI Medium 下该插件不产生任何呈现

## Answer

契约改动四处：`plugin.Validate` 改为「entry 与 ui 至少其一」；`discovery.Scan` 仅在 Entry 非空时做 exe 存在性检查（UI Entry 检查保留）；`serve.Start` 对无 entry 插件跳过 launch（无进程、无 Frame 路由、不占 registry）；`probePlugin` 跳过无 exe 的 echo 探测（mount 行保留，输出形状不变）。Web 侧本就以 UI 块存在性为键（`uiDirs`），零改动——`/api/plugins` 与 `/plugin-ui/` 原样服务 ui-only 插件。

测试：manifest 单测（ui-only 合法、双缺拒绝）；discovery 单测（uifix fixture 发现 + neither 拒绝）；cli seam 测试（`TestUIOnlyPluginMountsWithoutProcess`：discover 可见 → `-dump` 挂载 → `-call-plugin` 对无进程插件快速失败「not mounted」；`TestManifestWithoutEntryOrUIRejected`）；web 侧在既有计划里加 entry-less 插件断言 `/api/plugins` 暴露与 `/plugin-ui/` 服务。README 插件布局与 Manifest 字段表同步。

**额外（测试基建）**：本票首跑全量时 cli 包撞上 go 默认 10 分钟包超时（verbose 复跑 372s 通过，纯负载抖动）——套件每个测试都重新 go build 同一批二进制。修法：`buildPkg` 加进程级构建缓存（`sync.Map` 按 pkg 键，输出路径含 PID 防并行包冲突），cli 403s→60s、server→10s，超时隐患消除；属测试文件改动，零运行时影响。
