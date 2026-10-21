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
  "base_url": "https://api.deepseek.com/v1",
  "api_key": "",
  "models": [
    {
      "id": "deepseek-chat",
      "label": "DeepSeek Chat",
      "context_window": 65536
    },
    {
      "id": "deepseek-reasoner",
      "label": "DeepSeek Reasoner",
      "context_window": 65536
    }
  ],
  "default_model": "deepseek-chat"
}
```

`models` 是同一个 `base_url` 下的模型目录。`context_window` 用于上下文用量和预算；未知时可省略。
旧配置只有 `model` 时，启动会迁移成单模型目录。

启动优先级：

```text
环境变量
→ config.json
→ 内置默认值
```

支持的环境变量：

| 环境变量 | 字段 |
|---|---|
| `OPENAI_BASE_URL` | `base_url` |
| `OPENAI_API_KEY` | `api_key` |
| `OPENAI_MODEL` | `default_model` |

`config.json` 已被 `.gitignore` 排除，但保存设置时会把 `api_key` 以明文写入该文件（`0644`）。
请自行保护插件目录和文件权限；环境变量仍然优先。`config.get` 返回部分脱敏 key，
`llm.settings.get` 只返回是否已配置，不回显明文。

`base_url` 在文件、环境变量和 `config.set` 三条路径上都会归一化（去首尾空白和末尾 `/`）。

## 3. 前端交互

第一阶段没有 llm-openai 顶层 Component。

Host Settings 页面按统一 Schema 渲染，插件中心 → 设置 → `llm-openai`：

| 字段 | 控件 | 说明 |
|---|---|---|
| `base_url` | string | OpenAI-compatible 端点 |
| `api_key` | secret | 留空保持不变；保存后明文写入 `config.json` |
| `clear_api_key` | boolean | 保存时清除当前密钥 |
| `models` | list | 模型目录：ID、显示名、上下文窗口 |
| `default_model` | select | 选项来自 `models` 列表；新会话默认模型 |

读取调用 `llm.settings.get`，保存调用 `llm.settings.save`。插件负责持久化。每次 `llm.complete` 使用调用开始时的配置快照。修改不影响运行中的 Call，并从下一次 Call 开始生效。

模型目录在设置页维护；`llm.models.list/set` 仍然保留给 Capability 调用方。
会话左侧“新建会话”的模型下拉只调用 `llm.models.list`，不再提供独立的模型管理弹窗；
设置保存成功后，Host 会派发 `liteagent:settings-saved`，`session-history` 据此刷新模型下拉。
Session 创建时把选中的模型写入自己的元数据。`llm-openai` 不读取 Session。

空 Secret 表示不修改 API Key；要删除已保存的 Key，勾选“清除已保存的 API Key”后保存。

## 4. 文档

详细契约见 [communication.md](communication.md)。该文档也记录上游协议。
