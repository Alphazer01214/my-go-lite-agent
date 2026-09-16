# 01 — 双入口拆分：liteagent-cli / liteagent-server

**What to build:** 程序收敛为两个薄入口，共享同一内核与全部插件协议，行为与现状完全一致：liteagent-cli.exe 承载 CLI Medium 全部能力（-turn / -repl / -invoke / session 操作 / 原生命令）；liteagent-server.exe 承载 Web Medium（-serve，可与 -repl 组合运行）。build.ps1 产出含双 exe 与全部插件的 dist 布局。纯机械重构：web 资产、路由、PanelOp、SDK 原样搬运，协议不变。

**Blocked by:** —

**Status:** resolved

- [x] liteagent-cli.exe：REPL 多轮对话、/ 命令、-turn 均如旧
- [x] liteagent-server.exe：-serve 起 Web Medium，聊天 / trace / 插件面板如旧；-repl 组合可用
- [x] build.ps1 产出双 exe 的 dist 布局
- [x] 现有测试与回归全绿；protocol 保持 2

## Answer

共享运行时下沉 `internal/app`（flag 分发、mount 助手、CLI 渲染器、REPL、命令面、Web 启动器）；`cmd/liteagent-cli` = CLI 全量面（无 -serve），`cmd/liteagent-server` = `-serve` 必须 + `-repl` 组合；`cmd/host` 删除。主缝测试按面拆分：`web_test.go` → server，其余 → cli，`readline_test.go`（package main 内部测试）随 commandPlane 迁入 `internal/app`（package app）。git mv 保留测试文件历史。

**偏差一：全量套件在本票开工时即不绿。** HEAD 干净树 `go test ./cmd/host` 有 7 个失败（预存），根因是测试状态污染：各测试共享进程 CWD 下相对路径 `sessions/default.jsonl`，而 fakellm 的行为依赖上下文是否已含 tool 消息（有则不再发 tool call），同跑测试顺序与跨次残留都会破坏断言。修复：仓库已有但仅 2 个文件在用的 `hostEnv` 助手（SESSION_DATA_DIR 指向 per-test 临时目录）统一应用到全部挂载插件的 exec 点（cli 21 处、server/web_test 2 处）；presentation_test 自建的 isolateSessionEnv 并入 hostEnv。不挂 session 的诊断面（discover/assembly/roundtrip）未加 env（惰性无益）。修复后全量 `-count=1` 全绿。

**偏差二：** llm_openai 缺 key 测试的 env 改为 `append(hostEnv(t), "OPENAI_API_KEY=")` 叠加两个变量。

build.ps1 产出双 exe；README 二进制名与主缝测试位置同步更新。
