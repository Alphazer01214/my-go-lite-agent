# web-tools

`web-tools` 提供公网 HTTP 读取工具 `web_fetch`。

## 行为

- 只允许 `http` 和 `https`。
- 阻塞环回、私网、链路本地、组播、未指定和云元数据地址。
- DNS 解析后固定连接 IP，保留原始 Host 和 TLS ServerName。
- 每一跳重定向重新校验，最多 5 次。
- 默认响应上限为 1 MiB。
- 只接受 UTF-8 文本内容。

## 状态

`web_fetch` 已实现。`web_search`、`web_download` 和 `web_cancel` 暂未实现。

完整契约见 [communication.md](communication.md)。
