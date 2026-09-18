# plugin — Manifest 契约

规范名词：[Manifest](../../CONTEXT.md)、[hostFaces](../../CONTEXT.md)、[Autostart](../../CONTEXT.md)、[Plugin Dependency](../../CONTEXT.md)。

## 职责

磁盘 `plugin.json` 的类型、校验与存在性检查。**元数据唯一真源**；不是运行参数配置（config 走 Config Capability）。

Discovery 读入；Assembly / Host 校验。构建脚本**永不生成** Manifest。

## 文件

| 文件 | 内容 |
|------|------|
| `manifest.go` | 全部类型、Validate、文件系统检查 |
| `manifest_test.go` | 校验矩阵、BOM、UI、commands、slots |
| `manifest_autostart_test.go` | Autostart / dependsOn 解析 |

## 关键类型

### `Manifest`

| 字段 | 说明 |
|------|------|
| `Name` | `^[a-z0-9-]+$` |
| `Version` | 必填 |
| `Protocol` | `1..CurrentProtocol`（当前 **6**） |
| `Provides` / `Consumes` | Capability 名；声明与观测，不是 Host 路由表 |
| `Entry` | 可执行入口；可与 `UI` 至少有其一 |
| `TimeoutMs` | 单次 Frame 调用超时 |
| `Commands` | `[]CommandSpec{Name,Description,Usage}` |
| `UI` | `*UISpec`；UI-only 插件可无 Entry |
| `Autostart` | 作者默认挂载意图（根集） |
| `DependsOn` | 按**插件名**的硬依赖（挂载闭包） |
| `HostFaces` | `config` \| `commands` \| `ui`（与 provides 语义不同） |

### UI 面

- `UISpec`：`Entry`（`ui/*.js` ES Module）、`Assets`、`Mounts`、`Trust`（`full`|`isolated`）、`Pages`
- `UIMount`：`Page`/`Slot`/`Component`/`Props`
- `UIPage` / `UISlot`：加法贡献到 Layout（ADR-0012）
- 组件元素名必须以 **插件名为前缀**（`<pluginName>-*`）

### 校验要点

- Entry **或** UI 至少一个；UI-only 不得声明 provides
- `dependsOn` 不得自指；名字合法
- hostFaces 唯一且仅限三面
- Command 名无空白；与原生命令（`help`/`lp`/`refresh`/`exit`）冲突 → `ConflictsWithNativeCommand()`
- UI entry/assets 必须在插件目录下，禁止 `..` 与绝对路径
- `NormalizedEntry()`：Windows `\` → `/`

### 存在性 API

`LoadManifest(path)`、`EntryExists`、`ResolveEntry`、`UIEntryExists`、`UIAssetsExist` — 文件须存在且非目录。缺 README 仅 Discovery 警告，不阻断。

## 常量

```go
CurrentProtocol = 6
UISlots = []string{"top","bottom","left","center","right"}
ReservedCommandNames = []string{"help","lp","refresh","exit"}
```

## 错误策略

`Validate` fail-fast（单包硬失败）。Discovery 层会把坏目录记入 `ScanError` 并继续（ADR-0017 软失败在调用方）。

## 依赖

仅 stdlib。被 `discovery`、`assembly`、`serve`、`web`、`internal/app` 消费。

## 开发约定

1. 破坏 Manifest/UI 契约 → bump `CurrentProtocol` 并升级全部出厂插件。
2. Shell 宿主槽为 `top|bottom|left|center|right`（ADR-0031）；PanelOp 不得指向旧领域槽名。session 占 left（rail）与 center（chat|trace）。
2. hostFaces ≠ provides：faces 按插件名寻址；provides 是能力属主（唯一，tools 除外的历史语义已下放）。
3. Autostart 是作者意图；dependsOn 是硬闭包；consumes 是弱校验（degraded）。
4. 不要在此包引入 Host 路由或领域 payload 结构。
