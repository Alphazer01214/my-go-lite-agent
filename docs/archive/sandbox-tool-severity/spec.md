# Sandbox 按 Tool Severity 拦截

Status: ready-for-agent

## Problem

当前 sandbox 只靠 `permissions.json` 的 `{tool,path,action}` 规则 + 全局 `defaultAction`（缺省 allow）。插件作者无法在工具上声明风险；危险工具（shell/write）在无规则时直接放行，ask 链路几乎走不到。

## Solution

工具 schema 增加 **`severity`**（插件作者声明）：`low` | `medium` | `high`。

sandbox `policy.decide` 决策序：

1. 显式 rules（deny > ask > allow，现有语义不变）
2. **severity 映射**（可配置 `severityPolicy`，默认 `low→allow, medium→ask, high→ask`）
3. `defaultAction`

agent 在 callTool 前从 `tools.list` 取 severity 传入 decide。

## Tools 作者默认 severity

| tool | severity |
|------|----------|
| read_file / grep / glob / load_skill / web_search / web_fetch | low |
| write_file / edit_file | medium |
| shell | high |

## Issues

- 01-tool-schema-severity
- 02-sandbox-severity-policy
- 03-agent-pass-severity
- 04-tests-and-docs
