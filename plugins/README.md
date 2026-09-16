# plugins/

my-go-lite-agent 的插件索引。每个 `plugins/<name>/` 是一个可独立分发的扩展单元：**自己的 `plugin.json`（元数据真源）+ 可执行文件 + `ui/` 资源 + `README.md`**。构建脚本只做编译与拷贝，不再生成 manifest，因此 dist 与源码树不会漂移（ADR-0021）。

## 怎么被挂载

日常挂载真源不再是 Assembly 白名单，而是：

```
Manifest autostart（默认 false）作启动根
        │
        ├─ dependsOn（插件名，递归闭包，防环）
        └─ Agent Scheme 的 dependsPlugins ── Host ensurePlugins ──┐
                                                                 │
                          Discovery（plugins/ 下扫到的全部）──────┴─→ 挂载
```

`-assembly` 仍可用但已 deprecated（仅测试/调试）。发现 ≠ 挂载；`consumes` 只是弱声明，缺 provider 时插件标为 **degraded**（进程活着、不进 Capability registry、调用时报错，ADR-0022）。

## 状态一览

| 插件 | 状态 | 能力 | UI | 说明 |
|------|------|------|----|------|
| [agent](agent/README.md) | **核四件** autostart | `loop` | mode panel + 底栏 chip | Agent Loop；Agent Scheme（`chat`/`tool_calling`/`coding`）真源 |
| [session](session/README.md) | **核四件** autostart | `session` | rail / view / trace / 底栏 chip | Session Log（JSONL）；新会话首页 |
| [llm-openai](llm-openai/README.md) | **核四件** autostart | `llm` | 设置面 + 底栏 chip | OpenAI 兼容 provider（流式） |
| [context-manager](context-manager/README.md) | **核四件** autostart | `system-prompt` `context` | 底栏 chip | System Prompt 组装、prepare/compact/usage |
| [filetools](filetools/README.md) | 场景工具（scheme `coding` 拉起） | `tools` | — | 读/写/编辑/检索 |
| [shelltools](shelltools/README.md) | 场景工具（scheme `coding` 拉起） | `tools` | — | 跨平台 shell |
| [sandbox](sandbox/README.md) | 场景工具（scheme `coding` 拉起） | `policy` | — | allow / ask / deny 裁决；按 tool severity |
| [skill-manager](skill-manager/README.md) | 场景工具（scheme `coding` 拉起） | `skills` `tools` | — | Skill 发现、`$skill` 展开 |
| [project-context](project-context/README.md) | 场景工具（scheme `coding` 拉起） | `project-context` | — | AGENTS.md / CLAUDE.md |
| [webtools](webtools/README.md) | 场景工具（scheme `coding` 拉起） | `tools` | — | web_fetch / web_search |
| [echotool](echotool/README.md) | 示例（无 scheme 拉起） | `tools` | — | Presentation Card 最小样例 |
| [echo](echo/README.md) | **无用**（样例残留） | `echo` | — | 仅 CLI/夹具对端；产品路径无人挂载 |
| [uidemo](uidemo/README.md) | 示例（无 scheme 拉起） | — | sidebar 面板 | Panel Component 活文档 |
| [agentprobe](agentprobe/README.md) | **测试夹具，不发布** | — | — | `agent.request` 不变量 |
| [asyncsubllm](asyncsubllm/README.md) | **测试夹具，不发布** | `llm` | — | 异步 Subagent 回注 |
| [consumer](consumer/README.md) | **测试夹具，不发布** | `demo` | — | 星型路由 / consumes |
| [crashonce](crashonce/README.md) | **测试夹具，不发布** | `echo` | — | 崩溃后按需重启 |
| [emptytools](emptytools/README.md) | **测试夹具，不发布** | `tools`（空） | — | 无模型可见工具 |
| [promptreg](promptreg/README.md) | **测试夹具，不发布** | — | — | 运行时注册 Prompt Segment |
| [sessionprobe](sessionprobe/README.md) | **测试夹具，不发布** | — | — | 多会话 session.* |
| [slow](slow/README.md) | **测试夹具，不发布** | `slow` | — | 调用超时 |

### 「无用」的判定口径

- **测试夹具（8 个）**：`agentprobe` `asyncsubllm` `consumer` `crashonce` `emptytools` `promptreg` `sessionprobe` `slow`。
  没有 `plugin.json`，Go 测试按需编译；构建脚本显式列出但不安装。**不要在 dist 里手动放它们**——它们会污染 Discovery，且 `slow` / `crashonce` 会带来超时与重启噪声。
- **`echo`**：唯一真正冗余的插件。它只被 `consumer` 夹具与 CLI 手工 `-invoke` 使用；没有任何 `autostart`/scheme 会挂载它，产品路径上不会启动。保留仅为兼容既有测试。
- **`echotool` / `uidemo`**：示例/参考，随 dist 发布但不自动挂载；要在真实会话里用它们，把它们加进某个 scheme 的 `dependsPlugins`。

## 给插件作者

- `plugin.json` 是元数据唯一真源：`name` / `version` / `protocol` / `provides` / `consumes` / `entry` / `timeoutMs` / `description` / `commands` / `ui`，以及 `autostart` / `dependsOn`。
- `README.md` 为开发约定（Discovery 缺它只警告，不拒载）。建议写清：提供什么、怎么配置、`autostart`/`dependsOn` 是什么。
- UI 面板组件命名必须带插件名前缀（`<plugin>-*`），样式走 `--la-*` Design Token，交互经 `LiteAgent` 回插件。

参考：[CONTEXT.md](../CONTEXT.md) 术语、[docs/adr/](../docs/adr/) 决策。
