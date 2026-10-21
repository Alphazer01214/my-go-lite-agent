# tools

`tools` 是 Tool 注册与分发插件。它只管理 Tool Definition 和调用分发，不执行文件、命令或网络操作。

## 状态

当前实现 `tools.sync`、`tools.register`、`tools.unregister`、`tools.list` 和 `tools.call`。

## 依赖

无硬依赖。Tool Provider 通过 `tools.sync` 注册能力，缺少 Provider 时列表为空。

## 边界

- 注册表缓存到插件目录的 `registry.json`；tools 重启后恢复声明。
- Provider 重启后 Generation 变化，必须重新 `sync` 才能恢复。
- Tool 名称跨来源唯一。
- Provider 只能注册指向自身 Capability 的 Tool。
- `host` 和 `tools` 不能作为 Target Capability。
- 工具注册记录 Provider Generation；Provider 重启后必须重新 `sync`。
- 分发仍由 Host 按 `(capability, method)` 完成，tools 不直连 Provider。
- `risk` 是 0–100 的整数，越高越危险。

方法契约见 [communication.md](communication.md)。
