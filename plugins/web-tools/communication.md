# web-tools 通信契约

## 1. web.fetch

```json
{
  "url": "https://example.com/",
  "method": "GET",
  "headers": {"Accept": "text/html"},
  "max_bytes": 1048576,
  "timeout_ms": 30000
}
```

`url` 必填。`method` 只能是 `GET` 或 `HEAD`。`max_bytes` 最大 4 MiB。`timeout_ms` 最大 120000。

## 2. 结果

成功时 `content` 是响应正文。`meta`：

```json
{
  "final_url": "https://example.com/",
  "status_code": 200,
  "content_type": "text/html",
  "bytes": 1234,
  "truncated": false,
  "redirects": 0,
  "fetched_at": "RFC3339",
  "method": "GET",
  "duration_ms": 100
}
```

非 2xx 返回 `is_error: true`，`meta.code` 为 `upstream_error`。

## 3. 安全规则

- URL 不能包含用户信息。
- 每一跳重定向都重新检查协议和地址。
- 跨源重定向移除 `Authorization`、`Cookie` 和 `Proxy-Authorization`。
- `Host`、`Connection`、`Content-Length` 和 `Transfer-Encoding` 不能由调用方设置。

## 4. 错误码

| 错误码 | 场景 |
|---|---|
| `url_invalid` | URL 不合法 |
| `scheme_not_allowed` | 非 HTTP 或 HTTPS |
| `address_blocked` | 私网或保留地址 |
| `redirect_limit` | 重定向超过 5 次 |
| `content_type_unsupported` | 非文本内容 |
| `charset_unsupported` | 非 UTF-8 内容 |
| `upstream_error` | 请求失败、超时或非 2xx |
