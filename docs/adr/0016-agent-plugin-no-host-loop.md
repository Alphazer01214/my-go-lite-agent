# Agent 为进程外插件，Host 无内建 Loop

**Status: accepted（部分由 ADR-0030 supersedes）** — ADR-0030 进一步将 per-session 锁与 Cancel 从 Host 下放到 loop 插件；`agent.request`/`agent.inject` 暂缓（Deferred）。

Agent 是提供 `loop` Capability 的进程外 Plugin：经星型（ADR-0001）组合 llm、session、tools、system-prompt、context，执行 Agent Loop，并由 Agent 写回 Session Log 边界事实（turn/step/request_header/llm_usage）与 `run_subagent`/`MaxSteps` 策略。Host 不再持有默认 `runTurn` 或 Loop 策略代码；只保留 Discovery/Assembly/生命周期/Frame 路由、per-session 锁与 Cancel/running 状态，以及 `agent.request`/`agent.inject` 横切面（ADR-0002）。子 Turn 仍经 Host `loop.turn`（载荷含 allowSubagent/extraSystem），保证锁与取消单一真源。supersedes ADR-0003 与 ADR-0004 中「默认 Loop/Agent 在 Host」的折中。

「一切皆插件」要求 Agent 与 Session/LLM 同级可替换；内建 Loop 使默认路径与外置路径长期两张皮。外置代价是默认装配多一个进程与每步一跳 IPC——相对模型延迟可接受。无 Agent 插件时不能跑 Turn（见 ADR-0017 软失败）。
