# derive 投影 Tool Result Stub；Context Window 观测；自动压缩仍 deferred

长会话里历史 `tool_result`（尤其大文件读）会一直占满 Model Context。Session Log 仍是唯一真源且不删改；缩窗只能改投影。决策：`session.derive` 对超出 `fullToolResults`（默认 8）的更早 `tool_result` 投影为 Tool Result Stub，`tool_call` 保持全文以维持配对。LLM 插件经 `llm.info` 暴露 Context Window（用户可覆盖）；`context.prepare` 在估算占用超过 window 的 soft 比例（默认 0.80）时返回 `compactHint`，**默认 Loop 仍不自动 compact**（延续 ADR-0013）。filetools 默认 `read_file` limit 收紧为 500，减少进入 log 的体积。

代价：重放时模型看不到已 stub 的历史 tool 全文（需再调工具）；估算 token 非精确对账。自动 `context.compact` 触发与 provider token 对账 deferred。
