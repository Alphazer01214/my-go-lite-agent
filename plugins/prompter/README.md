状态：当前实现

# Prompter

Prompter 是模型请求装配器。
Session 只记录事实，Prompter 决定所有模型可见内容。

## 方法

| 方法 | 说明 |
|---|---|
| `session.create` | 补初始 `system_prompt`、`runtime_context` 两类 Fact |
| `llm.stream` | 装配 messages/tools 后 `Proceed` |
| `prompter.context.get` | 返回最近一次或派生的 LLM 可见上下文和用量 |
| `prompter.resources.list` | 返回 Skills / MCP 状态 |
| `prompter.resources.refresh` | 重新扫描 Skills、重连 MCP |
| `prompter.settings.get/save` | 读写设置 |
| `prompter.mcp.config.get/save` | 读写 MCP server JSON 配置 |
| `skill.info/read` | 向 Tools 暴露 `skill_read`，按需读取 SKILL.md |
| `mcp.info/call` | 向 Tools 暴露 `mcp_call`，调用 stdio MCP server |

## 上下文查看

`prompter.context.get` 返回：

- `source=live`：最近一次 `llm.stream` 实际装配的 messages/tools；内存缓存，按会话保留最近 8 份。
- `source=derived`：缓存不存在时，从 `session.facts` 和 `tool.list` 重新派生；只读，不写 Fact。
- `source=none`：还没有装配过上下文。

`usage.token_estimate` 是估算值；`context_window` 来自 `llm.models.list`，存在时同时返回百分比。
消息正文按单条 16 KiB、总量 1 MiB 截断，并通过 `truncated` 标记。

## Skills

- `skills_global_dir`：全局目录，绝对路径。
- `skills_workspace_dir`：workspace 内相对目录，默认 `.liteagent/skills`。
- 扫描 `<skills-dir>/<name>/SKILL.md`；frontmatter 读取 `name`、`description`，正文不主动注入。
- 初始只把 name/description 渲染到 runtime context；模型用 `skill_read` 加载完整 SKILL.md。
- workspace skill 覆盖同名 global skill；同一层重复名记为诊断。
- 两个目录都为空时，回退到旧配置的静态 `skills` 列表（只显示描述，不提供 `skill_read`）。

## MCP

当前只支持 stdio。

- `mcp_enabled` 控制总开关，`mcp_timeout_ms` 是全局超时。
- server 配置通过 `prompter.mcp.config.save` 写入 JSON：

```json
{
  "servers": [
    {
      "name": "filesystem",
      "enabled": true,
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
      "env": {"KEY": "value"},
      "cwd": "",
      "timeout_ms": 30000
    }
  ]
}
```

- 启用且有 `command` 的 server 会启动子进程，完成 initialize、`tools/list`，然后进入 connected。
- runtime context 只列 connected server 和工具名；模型通过通用 `mcp_call` 调用。
- `mcp_call` 参数：`server`、`tool`、`arguments`。
- server 状态、工具和错误显示在底部 MCP / Skills 面板；JSON 配置也在该面板编辑。
- 当前不实现 HTTP transport、per-tool 工具定义和自动重连；配置保存或 `resources.refresh` 会重连。

## UI

| 组件 | slot | 说明 |
|---|---|---|
| `prompter-context` | `right` | LLM 可见 messages、tools、snapshot 和用量 |
| `prompter-resources` | `bottom` | 收起时只显示 MCP / Skills 按钮；点击打开浮层，查看状态和 MCP JSON 配置 |

组件通过 `setData` 接收轻量状态推送，通过 `resync()` 重新拉取完整数据。
完整 messages 不放进 `ui.state`。

## Facts

| 类型 | Role | 内容 |
|---|---|---|
| `system_prompt` | `system` | 静态轨全文，`meta.hash` 记录内容哈希 |
| `runtime_context` | `user` | `<runtime_context>` 包裹的环境、skills、MCP |
| `context_snapshot` | `host` | 本次实际送出的 surface seqs、hash、token 估算；不投影 |

## 配置

`config.json` 可选；缺省使用内嵌的 `config.example.json`。

- `prompt_sections`、`skills`、`mcp_servers`：兼容旧配置。
- `skills_global_dir`、`skills_workspace_dir`、`mcp_enabled`、`mcp_timeout_ms`：通过 settings 修改，下一次调用生效。
- `mcp_servers`：通过底部面板的 JSON 编辑器修改。

`prompt_sections` 仍在进程启动时读取。

## 失败行为

- 配置非法：插件启动失败，消费链回落到 Provider。
- Skills/MCP 单项错误：写入 resources 状态，不阻断其他资源。
- 会话读取、渲染、Tool 获取或装配失败：返回错误。
- UI 状态推送失败（例如没有浏览器连接）忽略。

## 非职责

不持有会话历史；不调用模型；不定义模型 provider；首版不做上下文压缩和 MCP HTTP。
