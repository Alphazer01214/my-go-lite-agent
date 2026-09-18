# layout — Web Medium 页面与槽位真源

规范名词：[Layout](../../CONTEXT.md)、[Slot Role](../../CONTEXT.md)、[Panel](../../CONTEXT.md)。相关 ADR：0012、0010、0024。

## 职责

磁盘 `layout.json` 作为项目基座：定义 page 列表、slot 几何与 role。**不定义组件实现**。插件经 Manifest `ui.pages` **加法贡献**；Assembly 在合并结果上裁决 mounts。

## 文件

| 文件 | 内容 |
|------|------|
| `layout.go` | Doc / Validate / Merge（约 191 行） |
| `layout_test.go` | 加载、校验、合并测试 |

## 导出类型

| 符号 | 语义 |
|------|------|
| `Trust` | `"full"` \| `"isolated"`（仅 full 实现） |
| `Slot` | `ID`、`Role`、`Preferred`、`Region` |
| `Page` | `Slug`、`Title`、`Path`、`Slots` |
| `Doc` | 磁盘形状：`Trust` + `Pages` |
| `Merged` | 合并结果：`Trust`（默认 full）+ `Pages` |
| `Contribution` | `Plugin` 名 + `Pages []Page` |

## 函数

### `Load(path) (Doc, error)`

缺失文件 = **硬错误**（Web Server 不得无 Layout 启动）。与 Discovery 软失败对比鲜明。

### `(Doc).Validate()`

- pages 非空
- Trust ∈ {full, isolated}
- slug `^[a-z][a-z0-9-]*$` 且唯一
- 保留页 **`main` 必须存在**
- path 以 `/` 开头
- 每页 ≥1 slot，slot id 非空且页内唯一

### `Merge(base, contribs) (Merged, error)`

- 克隆 base
- 已有 slug：只追加**未知** slot id；重定义 base slot → error
- 空 Title 可填默认
- 新 page：`validateContribPage` 后追加
- 无 page 目标的独立 slot 忽略（v2）

## 项目基座 `layout.json`

ADR-0031：基座提供 **上/下 + 左/中/右** 五块通用区域；session 占左、中，chat/trace 由 session Panel Component 提供。

```json
{
  "trust": "full",
  "pages": [{
    "slug": "main", "title": "Main", "path": "/",
    "slots": [
      { "id": "top", "role": "panel", "region": "top" },
      { "id": "bottom", "role": "panel", "region": "bottom" },
      { "id": "left", "role": "panel", "region": "left" },
      { "id": "center", "role": "panel", "region": "center" },
      { "id": "right", "role": "panel", "region": "right" }
    ]
  }]
}
```

Shell 只负责五块区域的几何与槽位；内容归各插件 Panel Component（ADR-0024/0031）。

## 依赖与消费

- 仅 stdlib；**不** import `plugin`/`discovery`
- 消费方：`internal/app/web.go`（`-layout`，默认 `layout.json`）
- 与 `assembly.ManifestUIContributions` + `ResolveUIMounts` 串联后经 `/api/layout` 暴露

## 测试覆盖

缺文件失败；合法双页；缺 `main` 失败；加法合并（新 slot + 新 page）；重定义 base slot 拒绝。

## 开发约定

1. 贡献只加不改：base 页/slot 语义不可被插件覆盖。
2. Layout = 几何/role，不是组件代码。
3. 破坏磁盘 Layout 契约属 UI 契约破坏（ADR-0012，protocol 3 一脉）。
4. 导航由合并后的 pages 自动生成，Shell 不硬编码页表。
