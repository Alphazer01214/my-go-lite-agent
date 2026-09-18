# assembly — 挂载计划与 UI 裁决

规范名词：[Assembly](../../CONTEXT.md)、[Autostart](../../CONTEXT.md)、[Plugin Dependency](../../CONTEXT.md)、[Ensure Mount](../../CONTEXT.md)。相关 ADR：0021、0022、0023、0012。

## 职责

1. **日常挂载真源**：Autostart 根集 + `dependsOn` 闭包（ADR-0021）。
2. **UI 挂载裁决**：在 Manifest `ui.mounts` 上施加 Disable/Overrides/Winner。
3. **废弃白名单路径**：Assembly 文件 `Resolve` 保留作调试对照，不是日常入口。

## 文件

| 文件 | 内容 |
|------|------|
| `assembly.go` | Config/Plan/Load/Resolve（legacy） |
| `autostart.go` | `ResolveClosure`、`ResolveAutostart` |
| `ui.go` | EffectiveMount、ResolveUIMounts、ManifestUIContributions |

## 关键类型

| 类型 | 字段/语义 |
|------|-----------|
| `Config` | `Plugins []string` + 可选 `UI *UIConfig`（legacy + UI 裁决） |
| `UIConfig` | `Disable`（`plugin/component` 或 `plugin/page/slot/component`）、`Overrides` |
| `UIOverride` | Plugin/Component/Page/Slot/Props/Winner/Order |
| `Plan` | `Mounted`/`Unmounted`/`Missing`/`Rejected` |
| `Rejected` | 与原生 Command 同名（ADR-0008） |
| `EffectiveMount` | 裁决后：Plugin/Page/Slot/Component/Props/Winner/Order |

## 核心函数

### `ResolveAutostart(res discovery.Result) Plan`（日常路径）

- 根集：`Autostart==true` **或** UI-only（`Entry=="" && UI!=nil`）
- 再 `ResolveClosure` 展开 `DependsOn`
- 其余进 Unmounted

### `ResolveClosure(res, roots)`

单一依赖闭包算法：

- 前序：插件先于其依赖
- 未知名：根级 → `missing`；传递级 → stderr 跳过
- 自依赖跳过；环仍挂载双方
- 与原生 Command 冲突 → Rejected

Host `host.ensurePlugins` **复用**本函数（ADR-0023）。

### UI 路径

1. `ManifestUIContributions(plan)` — 收集已挂载插件的 `ui.pages` 加法贡献
2. `layout.Merge` — 与磁盘 layout 合并（见 layout 模块）
3. `ResolveUIMounts(cfg, plan)` — 施加 Disable/Overrides；默认 page=`main`；Winner 在 (page,slot) 上排他；排序 page/slot/Order/Component

## 错误与软失败

| 场景 | 行为 |
|------|------|
| `Load` I/O/解析失败 | 硬失败 |
| Resolve* | 永不 fail；missing/rejected 进 Plan 字段 |
| 未知传递依赖 / 环 | 警告并跳过（ADR-0017/0021） |
| `consumes` 缺失 | **不在本包处理**；挂载后 degraded，调用时再报错（ADR-0022） |
| ResolveUIMounts | 仅 override 缺 plugin/component 时报错 |

## 依赖与消费

- 输入：`discovery.Result`
- 输出：Plan → `serve.Start` / `internal/app.startMounted`
- UI：`internal/app/web.go` 串联 Merge 与 ResolveUIMounts
- Plugin Graph 读 Discovery **全量**，不是仅 Mounted（ADR-0025）

## 测试覆盖

挂载集合 + missing；原生命令拒绝（`help`）；Autostart 闭包；环；UI-only 恒为根；幽灵根；UI 默认 page、disable/override、winner 排他。

## 开发约定

1. Discover ≠ Mount ≠ Layout。
2. 硬依赖只有 `dependsOn`（启动）与 Scheme `dependsPlugins`（运行时 Ensure）。
3. 不要把 Assembly 文件恢复成日常白名单。
4. 不要在本包按插件目录名做 Host 特化调度（ADR-0030）。
