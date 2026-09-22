# Manifest - 插件

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
- `ui` **预留**；v1 不考虑 UI / HostFace

## WebUI Config（预留，v1 不使用）

v1 协议与运行时不处理 UI。以下字段仅供日后扩展，不参与 v1 路由或挂载。

### WebUI
- `entry` UI入口，例如 `main.js`
- `assets` 数组，包括css html
- `mounts`
- `pages`

### Mount
- `page` 装载页面名
- `slot` 装载位置, 包括 left, right, top, bottom, center
- `component` element 标签名
- `property` json

### Page
- `title`
- `slug`
- `path` 路由路径
- `slots` slot 数组

### Slot


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
    "entry": "main.js",
    "trust": "full",
    "mounts": [
      {
        "page": "main",
        "slot": "right",
        "component": "agent-mode-panel"
      }
    ]
  }
}

```