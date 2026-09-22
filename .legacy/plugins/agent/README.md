# agent

Default Agent Plugin（ADR-0016）：提供 `loop`，经 Host 组合 llm / session / tools / system-prompt / context。

## 提供

- Capability `loop`：`turn` / `cancel`
- Config Capability：`get` / `set` / `schema` / `reload`
- 命令：`/agent config [get|set defaultScheme=<name>]`

## Agent Scheme（config.json，与可执行文件同目录）

```json
{
  "defaultScheme": "tool_calling",
  "schemes": {
    "chat": {
      "dependsPlugins": [],
      "allowedTools": ["read_file", "grep", "glob"],
      "maxSteps": 8,
      "runSubagent": false,
      "todo": false
    },
    "tool_calling": {},
    "coding": {
      "dependsPlugins": ["filetools", "shelltools", "sandbox", "skill-manager", "project-context", "webtools"]
    }
  }
}
```

- `dependsPlugins`：进入 scheme 前经 Host `host.ensurePlugins` 拉起
- `allowedTools`：省略 = 不按名单过滤；`[]` = 无外部 tool；无 readOnly 门禁
- 热切换：`/agent config set defaultScheme=chat`（Web 命令面）或 `config.json` 的 `defaultScheme`，下一 Turn 生效；scheme 名写入 Session Log（`-scheme` flag 已删除，ADR-0027）

## UI

- `agent-mode-panel` —— 右上 Panels 栏的模式切换 chip
- `agent-status` —— Host 底栏信息区 chip（当前 scheme）
- `agent-settings` —— Settings 浮层里本插件的设置面（scheme 一览 + 切换）

## Manifest

- `autostart: true`（核四件之一）
- `dependsOn`: session, llm-openai, context-manager
