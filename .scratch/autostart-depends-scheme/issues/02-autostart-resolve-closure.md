# 02 - 无装配路径：Autostart �?+ dependsOn 闭包 Resolve

Status: done

**What to build:**

- 新入口逻辑（建�?`assembly` 包内新函数或 `discovery`/`host` �?`ResolveAutostart`）：�?`autostart==true` 为根，DFS `dependsOn`，visited 防环（警告跳过），未发现警告跳过，任意序产出 Mounted 列表�?- CLI/Server：`-assembly` 仍可传入但仅 `deprecated` 警告�?**忽略白名�?*；无 `-assembly` 时用闭包。保留旧 `assembly.Resolve` 供测�?学习（产品路径不调用）�?- 缝测试：零配置可挂核四件；dependsOn 拉起�?autostart 插件；环不挂死�?
**Done when:** `-plugins` only 启动�?`/lp` 见核闭包；旧 assembly 文件不影响挂载集�?