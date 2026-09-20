# Fixup 票据 — agent consumes 移除 tools（默认运行回归修复）

## 现象

默认装配（autostart 核四件：agent/session/llm-openai/context-manager）运行
`liteagent-cli -plugins dist\plugins -turn "你好"` 报：

```
warn: plugin agent consumes "tools" but no mounted plugin provides it (degraded)
liteagent-cli: agent loop: no plugin provides "loop" (mount an Agent plugin)
```

## 根因

Z2（8e7322e）按 spec「consumes 补全 system-prompt/context/session/llm/tools」
把 `tools` 加进 agent consumes。autostart 集没有 tools 提供方（filetools 等
仅由 scheme dependsPlugins 按需 ensurePlugins 拉起），reconcileConsumes
（ADR-0022：consumes 未满足 → degraded → provides 撤回）默认把 agent 判
degraded，`loop` 被撤出注册表 → 任何 Turn 在入口即失败。

基线（245c510）agent consumes 为 `[]`，默认聊天正常；此为用户可感回归。

## 依据

- agent 对无 tools 有显式降级：collectToolSchemas 失败返回 `(nil,false)`，
  Turn 注入「No tools are mounted in this assembly…」提示后继续纯聊天。
  tools 对 agent 是可选依赖，进 consumes 硬声明语义错误。
- ADR-0022：consumes 仅作声明与诊断，硬依赖由 dependsOn / scheme
  dependsPlugins 表达。
- 用户已确认修复方向：删 agent consumes 的 tools（保持最小装配）。

## 改动

- `plugins/agent/plugin.json`：consumes 移除 `tools`（留
  system-prompt/context/session/llm）。
- `dist/plugins/agent/plugin.json` 同步（运行时副本）。
- 本票据 + spec.md Z2 任务行修正标注。

## 校验

- 默认 `-turn "你好"` 不再报 loop 错误（degraded warn 消失，错误转为
  llm.complete 的配置/网络层，属环境要求）。
- `go build ./...` / `go vet ./...` / `go test ./...` 全绿。
- arch-check 12/12 不回退（C2/C6/C12 与 agent consumes 无依赖关系）。
