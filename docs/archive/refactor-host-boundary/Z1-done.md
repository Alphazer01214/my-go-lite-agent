# Z1 进展票据 — 零风险清理（已完成）

状态：Z1 全部完成，`go build ./... && go vet ./... && go test ./...` 全绿；
arch-check 3/12（新增翻绿 C8），已绿项 C3/C9 无回退。

## Commits（5 个逻辑单元）

1. `a8474d3` refactor(serve): Z1 删死代码 + 依赖闭包收敛为一处
   - 删 ExpandDepends / checkConsumes / emitRenderIntent / JSONToPairs /
     compactJSON / formatRunningLine / DebugEnabled / ListenAndServe /
     mdansi.Plain / mapToToolSchema / isReadOnlySchema / activeSchemeName /
     NewSessionID（约 600 行）
   - 收敛：导出 `assembly.ResolveClosure`，ResolveAutostart 与
     serve.EnsurePlugins 共用（原 walk 删除）
2. `f1530d2` refactor(fixtures): 8 个无 manifest 夹具目录迁 testdata/plugins/
   - 同步 9 处测试 build 路径、plugins/README.md、build 脚本注释
3. `a1f3433` cleanup(assembly): 删 examples/*.json（5 个）+ legacy
   assembly.Resolve 分支 → **翻绿 C8**
   - -assembly 变真正 deprecated no-op；cmd 层 2 个断言旧白名单行为的测试
     删除（与 ADR-0021 矛盾）；stubllm 夹具补 autostart
4. `f7727fe` docs(archive): 18 项 ready-for-agent issue 对账翻 resolved；
   17 个已完结目录归档 docs/archive/（用户已确认放宽原 12 个的出入）
   - 只留 refactor-host-boundary / web-ui-contract-v2（09 open）/
     webtools（无对应 ADR）
5. `64dce69` build(sdk): web/gen_sdk.go go:generate 同步 SDK 副本；
   scripts/shipped-plugins.conf 单一真源，build.sh/build.ps1 共享

## 记录在案的判断（供复核）

- EnsurePlugins 改走 ResolveClosure：对已挂载根的依赖缺失不再跳过，
  会补齐遗漏依赖（更符合 ensure 语义）；冲突插件被拒（与 Autostart 一致）。
- 评估的差异：Z1 原清单「12 个归档目录」对账后实为 17 个，已询问用户并确认。
- 未动：pluginsdk 4 个 emitter（Z3 接线）、-scheme flag（Z3）、web 的
  -assembly 测试引用（flag 保留为 no-op）。