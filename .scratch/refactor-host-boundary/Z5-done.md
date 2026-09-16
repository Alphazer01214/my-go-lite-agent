# Z5 进展票据 — 文档收尾（已完成）

状态：Z5 完成，arch-check 12/12 全绿，go test ./... 全绿，四篇 ADR 与代码行为一致。

## Commits

1. `(本 Zone commit)` docs(host): Z5 收尾 —— ADR-0018 todo 修订、README -scheme 示例改写、spec 状态翻新

## 改动明细

- **ADR-0018 修订**：todo provider 已改为 agent 内建 —— 工具提供方清单改为
  filetools/shelltools/skill-manager/webtools/echotool，新增修订段说明
  `todo`/`run_subagent` 是 agent 内建工具，不经 tools.list 注册、不进路由表，
  随 scheme 的 Todo/RunSubagent 开关暴露。
- **ADR-0027 措辞对齐（用户裁决）**：交互式入口 `/agent scheme <name>` → 
  `/agent config set defaultScheme=<name>`（对齐已落地的 REPL banner 与测试）。
- **README 对齐**：
  - chat scheme 描述补 dependsPlugins=filetools、无 todo（代码 config.go 实际值）
  - 快速开始 4 处 `-scheme` 示例全部改写：删除 flag 用法，改为「先设
    defaultScheme（config.json 或 REPL 热切换）再 -turn」
  - plugins/agent/README.md 的过期 `-scheme 覆盖 defaultScheme` 行删除
- **plugin-config/spec.md**：Status: in-progress → resolved（config 契约
  get/set/schema/reload + /refresh 广播已全部落地，ADR-0027 转正）。
- **issue 状态**：web-ui-contract-v2 仅 09-golden-path 保持 open（Playwright
  真浏览器脚本需本机安装，产品运行时零依赖，已在 Comments 记录）；其余 8 项
  均 resolved。refactor-host-boundary/spec.md 本次翻 resolved。

## 校验

- `go build ./...` / `go vet ./...` 全绿；`go test ./...` 全绿。
- arch-check 12/12 不回退。
- 四篇 ADR（0026..0029）与代码行为一致：hostFaces/protocol 4（0027）、
  自动压缩默认（0028）、订阅制（0029）、能力契约边界（0026）均已按 ADR 落地。
