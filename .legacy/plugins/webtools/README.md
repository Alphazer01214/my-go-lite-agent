# webtools

联网工具（Tools Plugin）：抓取网页与搜索。

## 提供

- Capability `tools`
  - `list` → `web_fetch`、`web_search`（均 readOnly）
  - `call` → 抓 URL 转文本，或搜索

## web_search 后端

- 配了 API key 时走百度千帆 AI 搜索（结构化 title / content / url / date 引用）；
  环境变量 `WEB_SEARCH_API_KEY` 优先于与可执行文件同目录的 `config.json` 里的 `apiKey`
- 未配 key 时回退 DuckDuckGo HTML（无需 key）

网络为尽力而为；失败返回 FrameError。

## Manifest

- 非 autostart；由 Agent Scheme `coding` 拉起
