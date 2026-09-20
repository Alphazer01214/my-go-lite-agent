# discovery — 插件目录扫描

规范名词：[Discovery](../../CONTEXT.md)。**发现 ≠ 挂载**；挂载归 Assembly。

## 职责

扫描插件根目录，得到「机器上有哪些可用插件」。只观察，不启动进程。

## 文件

| 文件 | 内容 |
|------|------|
| `discovery.go` | 全包（约 93 行） |
| `discovery_test.go` | 合法性矩阵、缺失根 |

## 导出 API

| 符号 | 语义 |
|------|------|
| `Found` | 一个插件目录 + 已校验 Manifest（`Dir`、`Manifest`） |
| `Result` | `Plugins []Found` + `Errors []ScanError` |
| `ScanError` | `Dir` + `Err`；`Error() = "<dir>: <err>"` |
| `Scan(root) Result` | 一级扫描 `<root>/<child>/plugin.json` |

## 扫描算法

1. `os.ReadDir(root)`；非目录跳过
2. 子目录存在 `plugin.json` 才算候选
3. `plugin.LoadManifest`；`Entry != ""` 时检查 `EntryExists`
4. `UI != nil` 时检查 `UIEntryExists` + `UIAssetsExist`
5. 缺 `README.md`：stderr 警告，不阻断（Plugin Readme 约定）
6. 有效插件按 `Manifest.Name` 排序
7. 无 `plugin.json` 的目录：忽略，不是错误

## 软失败（ADR-0017）

`Scan` **从不返回 error**。根读失败或单目录失败追加 `ScanError`，继续扫其余项。调用方打印错误并使用有效子集。

## 不变量

- 只观察，不挂载
- 一级目录（不递归）
- UI-only 插件（`Entry==""` 且 `UI!=nil`）是一等公民
- `protocol=1` 仍可挂载（未知 render kind 由 Host 丢弃）

## 依赖与消费方

- 依赖：`plugin`
- 消费：`assembly`（Resolve 输入）、`internal/app`（启动、`/refresh`）、`serve`（SetCatalog / Ensure Mount / Plugin Graph）

## 测试覆盖

合法 echo + UI-only；非法（缺 name、缺 entry、缺 UI asset、二者皆无）；非插件目录忽略；根缺失 → 1 error / 0 plugins；名字排序。

## 开发约定

1. 不要在此包做挂载决策或进程启动。
2. 不要递归扫描。
3. 保持软失败：调用方需要完整目录用于 Plugin Graph（ADR-0025），即使部分目录坏了。
