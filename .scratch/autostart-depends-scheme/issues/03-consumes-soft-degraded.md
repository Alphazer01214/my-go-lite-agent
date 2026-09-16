# 03 - consumes 软跳过与 degraded

Status: done

**What to build:**

- `serve.Start`：`checkConsumes` 改为收集 unmet �?警告；未满足的插�?**�?* 写入 Capability registry，标�?degraded（`/lp` 展示）�?- 路由：对 degraded 插件 Call 返回明确错误�?- 更新 lifecycle 测试：原 fail-loud consumes 用例改为「进程可�?+ 使用时报错」�?
**Done when:** consumer �?provider �?Host 不退出；`/lp` 可见 degraded；Call 失败信息可读�?