# Manifest - 插件

## 配置 manifest
`plugin.json` 是插件的清单文件，用于描述插件的元数据和功能。
主要包含如下字段：

### 身份 （Identity）
- `name` 插件的唯一标识符，建议使用插件的仓库名称。
- `version` 插件的版本号，建议使用语义化版本号（SemVer）。
- `description` 插件的描述，建议使用中文。
- `protocol` 待定
- `entry` 入口，默认由 name 直接推断（注意 windows 和 mac/linux）

### 能力 （Capabilities）
- `provides` 插件对外提供的能力
- `consumes` -> `requires` 插件需要别人提供的能力
- `autostart` 是否随 host 启动而启动，默认 `false`

### 交互 （Interaction）
- `commands` 插件对外提供的命令，例如 `/run`、`/stop` 等。
- `host_faces` host 提供的挂载点，作用类似能力，例如 `commands` 表示需要 host 提供命令， `config` 表示需要 host 提供配置入口。
- `webui` 见示例

## WebUI Config

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
  "protocol": 6,
  "autostart": true,
  "dependsOn": [
    "session",
    "llm-openai",
    "context-manager"
  ],
  "provides": [
    "loop",
    "agent-presets"
  ],
  "consumes": [
    "system-prompt",
    "context",
    "session",
    "llm"
  ],
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
      },
      {
        "page": "main",
        "slot": "bottom",
        "component": "agent-status"
      }
    ]
  },
  "hostFaces": [
    "config",
    "commands"
  ]
}

```