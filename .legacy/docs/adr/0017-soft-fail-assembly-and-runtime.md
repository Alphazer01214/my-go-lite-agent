# 装配与插件错误软失败

Discovery/Assembly/缺 Capability/mount 失败只打印错误、跳过该项，进程继续启动；用到缺失能力时再报错（如无 `loop`/`llm` 时 `-turn` 失败）。Turn 或插件调用失败不拖垮 Host：该 Turn 返回可见错误，可再次对话。Turn 内不做 Step 级自动续跑（ADR-0005：失败/取消不写半截 assistant）。

硬退出会让「其余插件已就绪」的场景整机不可用；假成功或静默降级又会掩盖装配错误。软失败 + 使用时报错在可用性与可诊断性之间折中。与 ADR-0016 配套：无 Agent 插件时 Host 仍可 `/lp`、探针与已就绪能力。
