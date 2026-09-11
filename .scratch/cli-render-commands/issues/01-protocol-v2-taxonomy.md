# 01 — protocol v2 + RenderKind 更名 + SDK 三方法

**Status:** resolved

## Answer

`protocol.Version=2`、`plugin.CurrentProtocol=2`。RenderKind 改为 `markdown_text|message_text|summary_text`；SDK `EmitMarkdownText`/`EmitMessageText`/`EmitSummaryText`；`expandable` 退役。未知 kind 丢弃并 warn。
