# 04 - Host ensurePlugins 横切�?
Status: done

**What to build:**

- Host 处理 `cap=host, method=ensurePlugins`（名称以实现缝为准，写入协议/CONTEXT 一致）：`{names:[]}` �?幂等挂载 + dependsOn 闭包；返�?`{ok, mounted, missing, failed}`�?- UI-only 插件：允许出现在 names/dependsOn（无进程，存在即成功）�?- CLI 缝测�?+ 文档一句�?
**Done when:** 已发现未挂载插件可被 ensure；未发现出现�?missing；重�?ensure 幂等�?