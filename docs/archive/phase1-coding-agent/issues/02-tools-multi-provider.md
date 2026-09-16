# 02 — Tools Capability 多提供方汇聚

**What to build:** Host 将 `tools` 从唯一属主改为可汇聚：

- 启动时多个插件 `provides: ["tools"]` 均可挂载；**工具名**全局唯一，冲突 fail-loud。
- `tools.list`：向全部 tools 提供方扇出并合并 schema；可附带 `readOnly` 等扩展字段原样透传。
- `tools.call`：按 `name` 路由到声明该工具的插件；未知工具返回既有 `unknown_tool` 类错误。
- 非 tools Capability 仍唯一属主。
- 文档/README 说明「多个 tools 插件可共存」。

**Blocked by:** 无

**Status:** resolved

- [x] 多提供方注册与同名冲突失败
- [x] list 合并、call 分路由
- [x] 主缝测试：两 tools 插件（如 filetools + echotool）一轮内均可被调用
- [x] 回归：单提供方行为不变

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- ADR-0018；推翻旧 issue 注释「多工具装在同一插件内」——有意演进。
- 05/08/09 依赖本票。
