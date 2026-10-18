# tools

`tools` 是 Tool 注册与分发插件。它只管理 Tool Definition 和调用路由，不执行文件、命令或网络操作。

## 状态

当前实现 `tools.register`、`tools.unregister`、`tools.list` 和 `tools.call`。

## 依赖

无硬依赖。Tool Provider 通过 `tools.register` 注册能力，缺少 Provider 时列表为空。

## 边界

- 注册表只存在内存。
- Tool 名称跨来源唯一。
- Provider 只能注册指向自身 Capability 的 Tool。
- `host` 和 `tools` 不能作为 Target。
- 路由仍由 Host 按 `(capability, method)` 完成。

方法契约见 [communication.md](communication.md)。
