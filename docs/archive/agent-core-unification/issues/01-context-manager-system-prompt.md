# 01 — Context Manager（system-prompt 组装）

**What to build:** Context Manager 插件 provides `system-prompt`：`registerSegment` / `registerContext` / `assemble`。Assembly 可带基础段；插件可运行时注册动态段。Loop 在 Turn 首个 Step 前 assemble，并把结果 append 成 Session Log 的 system 事实。

**Blocked by:** —

**Status:** resolved

- [x] 新插件提供 `system-prompt` Capability（方法：registerSegment / registerContext / assemble）
- [x] Prompt Segment 携带 name/order/text；按 `(order, name)` 稳定排序
- [x] Assembly 配置可注入基础段；运行时 registerSegment 可追加
- [x] registerContext 段在 assemble 时拼在 system 段之后（v1 并入同一文本）
- [x] 默认 Loop：Turn 开始（首个 Step 前）调用 assemble → `session.append` system 事实 → 再走 AgentRequest / llm
- [x] 无 system-prompt 提供方时行为明确（空 system，不 fail-loud）
- [x] 主缝测试：注册段后 assemble 文本可断言；一轮结束后日志含 system 事实且 derive 进 Model Context
- [x] 插件基于 `pluginsdk`，不手写 Frame 循环

## Answer

新增 `plugins/contextmanager`：内存段注册表，方法 `registerSegment` / `registerContext` / `assemble`；启动时从可执行文件旁的 `segments.json` 载入基础段。`serve` 增加 `SystemPromptCap` 与 `AssembleSystemPrompt`；`RunTurn` 在 append user 前先 assemble 并 `session.append` system 事实（无提供方则跳过）。CLI 增加 `-invoke-payload`，且 invoke 先于 turn 执行。主缝测试：`TestContextManagerAssemblesSystemPrompt`、`TestContextManagerRegisterSegmentViaStar`（fixture `plugins/promptreg`）、`TestTurnWorksWithoutContextManager`。

## Comments

- Capability 名用 `system-prompt`（ADR-0006），插件目录名 `contextmanager`。
- v1 不做 `{{var}}` 插值、scoped shadow、waterfall 钩子、tools 提供方。
- 不包住 `session.derive`；只产 System Prompt 文本。
- 压缩/surface replace 明确 Out of Scope（spec）。
- 基础段载体是插件目录 `segments.json`（Assembly 挂载该插件即注入）；完整 per-plugin Assembly 参数仍是后续票。
- `registerContext` v1 拼在段之后并入同一 system 文本。
