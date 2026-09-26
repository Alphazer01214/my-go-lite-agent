# Manifest - 插件

`plugin.json` 是插件包内 Host 注册的唯一依据。包结构与 `main.go` / `config.json` / `README.md` 职责见 [plugin-dev.md](plugin-dev.md)。

## 配置 manifest
`plugin.json` 是插件的清单文件，用于描述插件的元数据和功能。
主要包含如下字段：

字段命名一律 **snake_case**（与 [protocol.md](protocol.md) 一致）。路由与 dispatch **只看 `(capability, method)`**；插件名仅供日志、依赖图、观测。

### 身份 （Identity）
- `name` 插件的唯一标识符，建议使用插件的仓库名称；须与目录名一致。
- `version` 插件的版本号，建议使用语义化版本号（SemVer）。
- `description` 插件的描述。
- `protocol` 清单/UI 契约版本（与 Frame `version` 双层演进，见 protocol.md §1.3）。
- `entry` 入口，默认由 name 直接推断（注意 windows 和 mac/linux）

### 能力 （Capabilities）
- `provides` 插件对外提供的能力（Capability 名）；观测/依赖图用，路由真源是 `host.register` 上报的 `(capability, method)`
- `requires` 插件需要别人提供的能力（弱依赖；_Avoid: consumes_）
- `depends_on` 硬依赖的具体插件名（装配/依赖图；**不参与** Frame 路由）
- `autostart` 是否随 host 启动而启动，默认 `false`（v1 挂载为全量启动，此字段暂不参与调度）

### 交互 （Interaction）
- `commands` 插件命令元数据，执行入口仍是 `capability.method`
- `timeout_ms` 单次调用超时（v1 协议暂不强制超时）
- `ui` WebUI 组件贡献面；契约见 [webui.md](webui.md)（Host 注册与拼版用，**不参与** Frame 路由）

## WebUI（组件贡献）

设计全文见 [webui.md](webui.md) §5（注册）/ §7（字段）。对应 Go 类型：`internal/plugin/manifest.go` 的 `WebUI` / `WebMount` / `WebPage`。

### WebUI
- `entry` UI Entry（ES Module）路径，相对**插件包根**，例如 `ui/main.js`；模块内 `customElements.define`
- `assets` 数组，entry 之外需托管的 css/js 等相对路径（同相对包根）
- `mounts` 静态挂载列表
- `pages` 加法贡献页面（可选）
- `trust` `full`（默认）| `isolated`（预留）

### Mount
- `page` 装载页面名，可省略（默认 `main`）
- `slot` 装载位置：`top` | `bottom` | `left` | `center` | `right`，或本插件 `pages` 贡献的槽 id
- `component` 自定义元素标签名，须以 `<插件名>-` 为前缀（如 `session-rail`）
- `id` 可选；稳定面板 id（同 page+slot 内唯一），默认取 `component`。与运行时 PanelOp `id` 同语义
- `props` 初始属性（JSON object）；组件侧 `setData(props)`

### Page
- `title` 页面标题
- `slug` 页面标识（`[a-z0-9-]+`，不得占用内建 `main`）
- `path` 路由路径
- `slots` 本页私有槽 id 数组（`string[]`；无 role/preferred/region）

## 示例
`agent/plugin.json`
```json
{
  "name": "agent",
  "version": "0.1.0",
  "protocol": 1,
  "autostart": true,
  "depends_on": ["session", "llm-openai", "context-manager"],
  "provides": ["loop", "agent-presets"],
  "requires": ["system-prompt", "context", "session", "llm"],
  "entry": "agent.exe",
  "timeout_ms": 180000,
  "description": "Default Agent Plugin: composes llm/session/tools via the star (ADR-0016)",
  "commands": [
    {
      "name": "config",
      "description": "Show or set defaultScheme (chat|tool_calling|coding)",
      "usage": "/agent config [get|set defaultScheme=name]"
    }
  ],
  "ui": {
    "entry": "ui/main.js",
    "trust": "full",
    "mounts": [
      {
        "page": "main",
        "slot": "right",
        "component": "agent-mode-panel",
        "id": "mode",
        "props": { "scheme": "chat" }
      },
      {
        "page": "main",
        "slot": "bottom",
        "component": "agent-status"
      }
    ]
  }
}
```
