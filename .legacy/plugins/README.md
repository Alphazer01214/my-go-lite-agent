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

## 给插件作者

- `plugin.json` 是元数据唯一真源：`name` / `version` / `protocol` / `provides` / `consumes` / `entry` / `timeoutMs` / `description` / `commands` / `ui`，以及 `autostart` / `dependsOn`。
- `README.md` 为开发约定（Discovery 缺它只警告，不拒载）。建议写清：提供什么、怎么配置、`autostart`/`dependsOn` 是什么。
- UI 面板组件命名必须带插件名前缀（`<plugin>-*`），样式走 `--la-*` Design Token，交互经 `LiteAgent` 回插件。

参考：[CONTEXT.md](../CONTEXT.md) 术语、[docs/adr/](../docs/adr/) 决策。
