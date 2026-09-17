# Z3 进展票据 — Host 去特化（已完成）

状态：Z3 全部完成，arch-check 12/12 全绿，go test ./... 全绿。

## Commits

1. `99a9671` refactor(host): Z3a 去特化 —— wire 类型统一 + 展示信号订阅化 + 收口导出面
   - 翻绿 C4 / C10 / C11
   - pluginsdk 成为 PanelOp/SummaryPair/RenderIntent/Card 契约真源
   - On* 单槽钩子 → 订阅制；OnToolApproval → RegisterApproval 先应答者生效
   - 22 个导出方法收口（内部化 / 删除 / CallByCap / CallByFace）
2. `cd5b12b` refactor(host): Z3b 去特化收尾（注释级：CancelTurnOn 说明、validatePanelOp 出处）

注：Z3 的其余大改动（C1 常量契约化、C5 Start soft-fail、C6 chat+filetools、
C12 示例挂载、workspace.resolve 下放、-scheme 删除、探针契约化）与 Z3a 同批
提交（Z3a 实际含全部工作区改动，message 覆盖了主体）。

## 主要决策（供复核）

- CallByFace 是包级函数（非 Server 方法）—— 不扩 C4 allowlist，且校验
  hostFace 声明使「绕过注册表」编译期不可能
- pluginsdk 增加 Host 能力契约常量（session/agent/llm/context/loop/tools/
  system-prompt + ProbeCap/ProbeMethod）—— C1 的解法：字面量在契约包，
  Host 只用别名
- Start 软失败化：单插件 launch 失败 warn 继续（ADR-0017），重名工具在
  tools.list 使用时 fail-loud（TestDuplicateToolNameFailsLoud 已改）
- 示例插件（echotool/echo/uidemo）随 coding scheme 挂载（C12），默认
  tool_calling 保持轻量（避免破坏无示例装配的测试）
- workspace.resolve 由 filetools 实现（遍历留在插件，Host 只转发）
- CancelTurnOn 保持 fire-and-forget req（spec 括号允许形态），已在注释标注