# 09 — Playwright 黄金路径 + 全量回归

**What to build:** dev-only e2e：开页→发送→回复→切会话→trace；go test ./... 绿。

**Status:** open

## Comments

- 2026-09: HTTP 面已由 `cmd/liteagent-server/web_test` 覆盖（shell、/api/message SSE、/api/command、session.query）。真浏览器 Playwright 脚本待后续（需本机安装 Playwright；产品运行时仍零依赖）。
