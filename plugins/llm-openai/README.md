# llm-openai

`llm-openai` 是无状态的 OpenAI-compatible LLM 适配器。它接收 messages 和 tools，返回最终结果，并在流式模式下发送 delta。

## 1. 能力

| Capability | 说明 |
|---|---|
| `llm` | Completion 和配置 |

`llm-openai` 不读取 Session，不编排 Turn，不渲染 WebUI 正文。

## 2. 配置

```json
{
  "models": [
    {
      "id": "deepseek-chat",
      "label": "DeepSeek Chat",
      "context_window": 65536,
      "base_url": "https://api.deepseek.com/v1",
      "api_key": ""
    },
    {
      "id": "gpt-4o",
      "label": "GPT-4o",
      "context_window": 128000,
      "base_url": "https://api.openai.com/v1",
      "api_key": ""
    }
  ],
  "default_model": "deepseek-chat"
}
```

`models` 是模型目录，每个条目自带 `base_url` 与 `api_key`：模型 ID 决定请求打到哪个供应商。
不同供应商的模型可以在同一个目录里并存，同一供应商的多个模型重复填写端点即可。
`context_window` 用于上下文用量和预算；未知时可省略。

旧配置（顶层 `base_url` + `api_key` + 不带端点的 `models`）启动时自动迁移：
顶层端点和密钥填进没有自己端点的条目。旧字段只有 `model` 时迁移成单模型目录。

启动优先级：

```text
环境变量
→ config.json
→ 内置默认值
```

支持的环境变量：

| 环境变量 | 作用 |
|---|---|
| `OPENAI_MODEL` | 指定默认模型；目录里没有时追加一条 |
| `OPENAI_BASE_URL` | 覆盖默认模型条目的 `base_url` |
| `OPENAI_API_KEY` | 覆盖默认模型条目的 `api_key` |

一组环境变量只描述一个端点，所以它们只作用于默认模型那一条；
其余条目仍按 `config.json` 里各自的端点路由。

`config.json` 已被 `.gitignore` 排除，但保存设置时会把 `api_key` 以明文写入该文件（`0644`）。
请自行保护插件目录和文件权限；环境变量仍然优先。`config.get` 返回部分脱敏 key，
`llm.settings.get` 每行返回脱敏后的 key，均不回显明文。

密钥在各方法里是三态字段：不传表示保留同 ID 条目的原密钥，空串表示清除，
其它值覆盖；传回读到的脱敏值等价于保留。`base_url` 在文件、环境变量和 `config.set`
三条路径上都会归一化（去首尾空白和末尾 `/`）。

## 3. 前端交互

第一阶段没有 llm-openai 顶层 Component。

Host Settings 页面按统一 Schema 渲染，插件中心 → 设置 → `llm-openai`：

| 字段 | 控件 | 说明 |
|---|---|---|
| `models` | list | 模型目录：ID、显示名、上下文窗口、Base URL、API Key |
| `default_model` | select | 选项来自 `models` 列表；新会话默认模型 |

每行一个模型，各自绑定自己的端点和密钥，可以混配多个供应商。
密钥框显示脱敏值：保持脱敏值不变则保留原密钥，清空则删除，填新值则覆盖。

读取调用 `llm.settings.get`，保存调用 `llm.settings.save`。插件负责持久化。每次 `llm.complete` 使用调用开始时的配置快照。修改不影响运行中的 Call，并从下一次 Call 开始生效。

模型目录在设置页维护；`llm.models.list/set` 仍然保留给 Capability 调用方，
输出不含端点和密钥。会话左侧“新建会话”的模型下拉只调用 `llm.models.list`，不再提供独立的模型管理弹窗；
设置保存成功后，Host 会派发 `liteagent:settings-saved`，`session-history` 据此刷新模型下拉。
Session 创建时把选中的模型写入自己的元数据。`llm-openai` 不读取 Session。

`llm.complete` 的 `model` 必须是目录里的 ID：ID 决定路由到哪个端点。
省略时用 `default_model`；目录里没有的 ID 返回 `bad_arguments`。

## 4. 文档

详细契约见 [communication.md](communication.md)。该文档也记录上游协议。
