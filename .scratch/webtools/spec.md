# webtools：web_fetch / web_search

Status: ready-for-agent

## Problem Statement

Coding agent 缺网络面：无法读文档页、无法检索。需要可配置密钥的只读 web 工具，并接到 coding 装配。

## Solution

独立插件 `plugins/webtools`，provides `tools`：

| 工具 | 输入 | 行为 |
|------|------|------|
| `web_fetch` | `url`, `timeoutMs?` | GET，HTML→纯文本；非 HTML 截断返回 |
| `web_search` | `query`, `limit?`, `timeoutMs?` | 配置了 apiKey → 百度千帆 AI Search（title/content/url/date）；否则 DuckDuckGo HTML 结果页解析 |

约束：

- 仅 `http`/`https`；拒绝 localhost / 私网 / link-local（轻量 SSRF 护栏）
- 超时默认 20s；body ≤ 512KB；输出 ≤ 24KB
- schema `readOnly: true`（可并行）
- 无第三方依赖（stdlib）

## 配置

- `config.json`（与可执行文件同目录）`{"apiKey": "..."}` — Baidu Qianfan API Key
- 环境变量 `WEB_SEARCH_API_KEY` 优先于 config.json
- 未配置 key 时降级 DuckDuckGo HTML（无需密钥）

## Out of Scope

- 浏览器渲染 / JS 页面
- 其他付费 Search API（Brave/Serper 等）
- 网页写入、表单提交

## Assembly

`examples/coding.json` 已含 `webtools`。
