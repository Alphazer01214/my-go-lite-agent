# Agent Framework 实现记录

本文记录 `plugins` 分支上的第一轮完整实现。设计输入见 [Agent Tools 完整设计](2026-10-18-agent-tools-full-design.md) 和 [Cordis 对比与插件设计](../dynamic-composability.md)。

## 1. 已实现

| 插件 | 能力 |
|---|---|
| `tools` | 注册、注销、列表、分发和来源校验 |
| `file-tools` | 读取、写入、列表、状态、搜索、Glob、替换和补丁 |
| `sandbox` | 默认风险策略、Workspace 规则和 Pipeline Proceed |
| `shell-tools` | 同步进程执行、输出限制、超时和进程树取消 |
| `web-tools` | SSRF 安全的 HTTP Fetch |
| `model-context` | System Prompt、Tool Definition、Snapshot 和压缩 |
| `subagent` | 同步子 Turn、深度限制、并发限制和取消 |

SDK 增加 `Request.From`。Host 修复了 Consumer 先启动时抢占 Provider Route 的问题。

## 2. 发布面

- `scripts/shipped-plugins.conf` 收录全部 10 个插件。
- Windows 和 Unix 构建脚本都复制插件 Manifest、README 和配置示例。
- 新安装使用 `config/permissions.example.json` 的默认策略。
- 每个插件都有 `plugin.json`、`README.md` 和 `communication.md`。

## 3. 验证

已执行：

```text
go test ./...
go vet ./...
go test -race ./...
powershell -File scripts/build.ps1 -Clean -Verify
```

发布构建产出 11 个可执行目标。Host 冒烟验证覆盖：

1. 全部插件进入 `active`，Tool Provider 注册成功。
2. `tools.list` 返回 11 个 Tool。
3. `tools.call → file.read` 返回文本、摘要和截断元数据。
4. 高风险 `file.write` 默认返回 `sandbox_denied`。
5. 显式 Workspace 规则放行高风险写入。
6. `shell_execute` 经 Sandbox 执行并返回退出码。
7. `web_fetch` 读取公网文本。
8. `model-context.build` 注入 System Prompt、Tool 和风险说明。
9. Session 写入 `context_snapshot` Fact。

## 4. 当前边界

- `web_search`、`web_download` 和 `web_cancel` 尚未实现。
- Shell 只有同步执行，没有后台任务表。
- Subagent 只有同步调用。
- `model-context.inspect` 只覆盖当前进程。
- Sandbox 没有 UI 确认流程。
- Workspace 边界不是操作系统级安全沙箱。

## 5. 后续方向

1. 增加后台 Shell 任务与状态查询。
2. 增加 Web Search Provider 和下载工具。
3. 增加 Sandbox 待确认状态与 UI。
4. 增加 Model Context 的 Token 精确估算和策略化压缩。
5. 增加 Subagent 并发编排、结果聚合和父子 Session UI。
