# Presentation 三分类改为 markdown_text / message_text / summary_text

主窗口渲染意图从 `markdown | expandable | message` 替换为 `markdown_text | message_text | summary_text`；`expandable` 退役，其「摘要 + 详情」语义由 `summary_text`（title + 保序 pairs + detail）承担。`protocol.Version` bump 到 2，SDK 直接换名为 `EmitMarkdownText` / `EmitMessageText` / `EmitSummaryText`，不留旧名 alias。

四分类会加重 Render Medium 与插件作者的认知负担；仓库内尚无外部插件用户，wire breaking 的代价可控。`Card` 路径不变。
