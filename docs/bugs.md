

1. shell tool 经常执行错误
2. 在设置中修改模型，会话无法及时修改。仅模型名出问题，base_url api_key 及时刷新了：http 400: {"error":{"message":"The supported API model names are deepseek-flash, deepseek-v4-pro, but you passed mimo-v2.6-flash. (request_id: 3d3a5999-112d-4e53-b945-4ed56fb70eef)","type":"invalid_request_erro… (3)
3. tool call 出现 http 400: {"error":{"message":"Messages with role 'tool' must be a response to a preceding message with 'tool_calls' (request_id: db71977d-90c3-44e3-aeca-6ab67e01edc9)","type":"invalid_request_error","param":nu… 这是由于 tool call message 没有按照指定的方式传到供应商

