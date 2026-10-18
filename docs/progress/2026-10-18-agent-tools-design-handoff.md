# Agent 常用 Tools 设计交接

本文是下一轮 Tools 设计的交接输入。当前只整理设计，不实现 Tool Provider。

## 1. 当前基础

平台已经完成以下切片：

- Provider 生命周期；
- Binding Generation；
- Context Call；
- Cancel Frame；
- Frame Priority 流水线。

Host 只负责 Frame 路由、执行上下文、取消和流水线。Tool 业务由插件实现。

Agent 已经使用两个可选能力：

- `tools.list`；
- `tools.call`。

当前最小组合没有 `tools` 插件。下一轮先定义完整 Tool 面，再实现 `tools` 插件和 Tool Provider。

## 2. 统一 Tool 模型

`tools` 是 Tool 注册与分发插件，不是通用 Service Broker。

一个 Tool Definition 包含：

| 字段 | 含义 |
|---|---|
| `name` | 模型看到的 Tool 名称 |
| `description` | 模型看到的说明 |
| `input_schema` | 参数 JSON Schema |
| `source_plugin` | 提供方插件名 |
| `target_capability` | Host 分发 Capability |
| `target_method` | Host 分发 Method |
| `risk` | `low`、`medium` 或 `high` |

`source_plugin` 由 Host 注入的 `from` 自动取得。插件不能伪造来源。

`target_capability`、`target_method` 和 `source_plugin` 不发送给模型。模型只看到名称、说明、参数 Schema 和风险级别。

## 3. 工具接口

`tools` 提供四个能力：

| 能力 | 用途 |
|---|---|
| `tools.register` | 注册 Tool |
| `tools.unregister` | 注销 Tool |
| `tools.list` | 返回模型可见的 Tool Definition |
| `tools.call` | 根据 Tool 名称调用目标 Method |

`tools.call` 的输入建议为：

```json
{
  "name": "file.read",
  "arguments": {"path": "README.md"},
  "workspace": "./workspace",
  "call_id": "tool-1"
}
```

`tools` 将目标调用交给 `target_capability.target_method`。目标调用统一使用以下包络：

```json
{
  "arguments": {},
  "workspace": "./workspace",
  "call_id": "tool-1"
}
```

目标 Tool Provider 自己解释 `arguments`。`tools` 不理解文件、Shell 或网络业务字段。

## 4. 常用工具目录

### 4.1 File Tools

建议首批能力：

| Method | 用途 | 风险 |
|---|---|---|
| `file.read` | 读取文本文件 | `low` |
| `file.write` | 写入文本文件 | `high` |
| `file.list` | 列出目录 | `low` |
| `file.stat` | 读取文件元数据 | `low` |
| `file.search` | 搜索文件内容 | `low` |
| `file.glob` | 按模式查找路径 | `low` |
| `file.replace` | 替换文本片段 | `high` |
| `file.patch` | 应用结构化修改 | `high` |

所有路径使用 Workspace 相对路径。Provider 必须拒绝绝对路径、`..` 逃逸和符号链接越界。

文件读取必须支持最大字节数、行范围和二进制检测。写入、替换和补丁必须返回修改摘要。

### 4.2 Shell Tools

建议能力：

| Method | 用途 | 风险 |
|---|---|---|
| `shell.execute` | 执行命令 | `high` |
| `shell.cancel` | 取消执行 | `medium` |
| `shell.status` | 查询执行状态 | `low` |

Shell Provider 使用 Go `context.Context`。取消时必须终止整个子进程组。

执行参数至少包含：

- `command`
- `args`
- `cwd`
- `env`
- `timeout_ms`
- `max_output_bytes`

输出至少包含：

- `exit_code`
- `stdout`
- `stderr`
- `truncated`
- `started_at`
- `finished_at`

### 4.3 Web Tools

建议能力：

| Method | 用途 | 风险 |
|---|---|---|
| `web.search` | 搜索公开网页 | `medium` |
| `web.fetch` | 读取网页或 API 响应 | `medium` |
| `web.cancel` | 取消网络操作 | `low` |

Web Provider 必须设置连接超时、读取超时、响应大小上限和内容类型限制。默认禁止访问环回地址、私有地址和云元数据地址。

`web.fetch` 返回内容时必须保留最终 URL、状态码、内容类型和截断标记。

### 4.4 Agent 辅助 Tools

第二批可以考虑：

| Method | 用途 | 风险 |
|---|---|---|
| `file.move` | 移动或重命名文件 | `high` |
| `file.delete` | 删除文件 | `high` |
| `shell.status` | 查询后台任务 | `low` |
| `web.download` | 下载文件 | `high` |
| `git.status` | 查看仓库状态 | `low` |
| `git.diff` | 查看差异 | `low` |

删除、移动、下载和 Shell 必须交给 Sandbox 审核。首批设计不承诺这些能力已经实现。

## 5. 风险和 Sandbox

| 风险 | 典型操作 | 默认策略 |
|---|---|---|
| `low` | 读取、列表、搜索 | 直接执行 |
| `medium` | 网络读取、进程查询 | 记录并执行 |
| `high` | 文件修改、命令执行、下载 | Sandbox 确认 |

Sandbox 是 Consumer，不是 Tool Provider。它至少检查：

- `tools.call`
- `file.write`
- `file.replace`
- `file.patch`
- `shell.execute`
- `web.fetch`
- `web.download`

Sandbox 可以允许、修改、拒绝或转为待确认状态。它不负责实际执行文件、Shell 或网络操作。

## 6. 取消和终态

所有异步 Tool 必须使用 Context Call 和 Cancel Frame。

每个 Tool Call 只有一个终态：

- `completed`
- `failed`
- `cancelled`
- `deadline_exceeded`

`shell.cancel` 和 `web.cancel` 是显式业务取消入口。它们不替代 Go Context 的调用级取消。

旧 Generation 的取消不得影响重启后的新 Provider。

## 7. 下一轮设计产出

下一轮先产出设计，不直接写 Provider。设计文档至少包含：

1. 每个 Tool 的 Method 清单；
2. `arguments` JSON Schema；
3. 返回值和错误码；
4. 风险级别；
5. 取消语义；
6. Workspace 和路径约束；
7. Provider Manifest 示例；
8. Sandbox 检查点；
9. 单元测试和集成测试矩阵。

## 8. 建议验收标准

- 每个 Tool 都有稳定名称和参数 Schema；
- 每个 Tool 都标明风险；
- Tool Registry 能注册、注销、列出和调用；
- `source_plugin` 不可伪造；
- 目标 Capability 和 Method 不暴露给模型；
- 文件路径不能逃逸 Workspace；
- Shell 取消能终止子进程；
- Web 请求有超时、大小和地址限制；
- 每个异步 Call 只有一个终态；
- `tools.call` 经过 Sandbox 高危检查；
- 设计文档、插件文档和测试计划同步。

## 9. 交接状态

当前代码尚未新增 `tools` 插件，也尚未实现 File、Shell、Web Provider。下一步是完成全套 Agent 常用 Tools 的详细设计，再按 File、Shell、Web、Sandbox 的顺序实现。

详细设计见 [Agent Tools 完整设计](2026-10-18-agent-tools-full-design.md)。
