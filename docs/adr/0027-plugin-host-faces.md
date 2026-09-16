# 插件面声明：config / commands / ui 转正

**Status: accepted**

`config`、`commands`、`ui` 三个面由 Host 按插件寻址调用，但从未在 Manifest 里声明：13 个出厂插件中 7 个存在「声明 ≠ 实际注册」的差异——`config` 被 5 个插件实现、`commands` 4 个、`ui` 1 个，而 `provides` 中出现 0 次。后果是这三面对 Plugin Graph（只读 Manifest）、`consumes`、degraded 全部不可见，Host 只能靠按名后门（`CallCommand` / `CallUIAction` / `CallByPlugin`）触达，并在 `/refresh` 里吞掉 `method_not_found`。

不能简单加进 `provides`：`registerProvides`（serve.go:321）对非 `tools` 能力冲突 fail-loud，5 个插件同时声明 `config` 会让 `Start` 直接报 `capability "config" provided by both ...` 起不来。语义上二者也不同——能力是「唯一属主、按能力名路由」，这三个面是「每个插件都可实现、按插件寻址」。

**决策：Manifest 新增 `hostFaces` 数组**，取值 `config` | `commands` | `ui`。

```json
{ "name": "session", "version": "1.0.0", "protocol": 4,
  "provides": ["session"], "hostFaces": ["config", "commands"] }
```

随之收窄：

- `Call` / `CallByPlugin` / `CallCommand` / `CallUIAction` 不再导出给 `web` 与 `internal/app`，由 Host 内部按 `hostFaces` 派发。这样「绕过注册表」在编译期就不可能。
- `/refresh` 的 `config.reload` 只广播给声明了 `config` 面的插件，不再遍历全部插件并吞错误。
- Plugin Graph 把 `hostFaces` 也画成节点与边，三个面立刻可见；`consumes` 可对 `hostFaces` 生效。

同时停止为单个插件定制读取路径（承接 ADR-0026）：

- agent 插件增 `provides: ["agent-presets"]`（可选实现），对外暴露 `defaultScheme` 与 `schemes`。Plugin Graph 的 `scheme` 边（ADR-0025 要求）改由 `CallByCap("agent-presets", "get")` 取数；未实现时图不画 scheme 边。
- Web Settings 继续走 README 已描述的通用机制（凡实现 `config.schema` / `config.get` 的插件自动出现在列表中），删除 `web/server.go:873` 对 agent 的定制读取。
- CLI 与 server 的 `-scheme` flag 删除，切换改由用户在命令面输入 `/agent scheme <name>`——CLI 因此完全不必知道 scheme 概念。

Manifest `protocol` 升至 4：带 `hostFaces` 的清单在旧 Host 上被拒载（承接 ADR-0012 的版本纪律）。允许破坏性变更，不留旧 manifest 兼容路径。
