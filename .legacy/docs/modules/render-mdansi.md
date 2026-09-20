# render/mdansi — Markdown → ANSI

规范名词：[Render Medium](../../CONTEXT.md)、[Presentation](../../CONTEXT.md)。相关 ADR：0007。

## 职责

把 Markdown 渲染为带 ANSI 样式的终端文本，供 **CLI Medium** 使用。纯展示工具，无 Host/插件逻辑。

Web Medium 使用各插件 Panel 内的 JS markdown 渲染，不经过本包。

## 文件

| 文件 | 内容 |
|------|------|
| `mdansi.go` | `Render`、`Indent` |
| `mdansi_test.go` | 基础/代码块/表格链接/Indent |

## 导出 API

### `Render(src string) string`

经 goldmark（CommonMark + Table 扩展）解析后输出 ANSI 文本：

| 结构 | 样式 |
|------|------|
| 标题 | cyan/bold |
| 代码块 | dim + 盒线 `┌─│└─` |
| 表格 | `│ cell │` |
| 引用 | 灰色 `│` 前缀 |
| 列表 | 绿色 `•` / `N.` |
| 分隔线、粗体、斜体、行内码、链接、图片 | 常规终端习惯 |

折叠连续三空行；保证末尾换行。

### `Indent(s, pad string) string`

非空行前缀 `pad`（Summary detail 缩进用）。

## 依赖

- `github.com/yuin/goldmark` — **允许的解析库例外**（核心零第三方规则）
- 唯一 importer：`internal/app/paint.go`

## 消费路径

```text
插件 EmitMarkdownText / RenderIntent{Kind:markdown_text}
  → serve Event topic=presentation
  → turnRenderer.onRender
  → mdansi.Render → 终端
```

## 测试

4 个用例；断言前剥 ANSI 转义。无像素级/终端仿真测试。

## 开发约定

1. 保持无状态纯函数；不做 I/O。
2. 新 goldmark 扩展需评估终端可读性与体积。
3. 不要让 Host/插件直接依赖本包——仅 CLI Medium 绘制层使用。
