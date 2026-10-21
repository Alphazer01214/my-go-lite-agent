# Session、Workspace 与多会话实现记录

> 状态：当前实现
> 说明：Session 恢复缺陷、turn id 格式、Workspace 绑定不可变与 `session.close` 不等于取消，仍然成立。

本文记录 Session 恢复修复和多会话标签实现。Workspace 绑定和 Sandbox 锚定的后续契约见 [消费链组合方案](../../design/composition-model-context-tools-sandbox.md) 第 7 节的 `meta` 字段集。

## 1. Session 根因

发现两个恢复相关缺陷：

1. `recover` 打开 JSONL 时缺少 `O_APPEND`。插件重启后的首次写入会从文件头覆盖旧 Fact。
2. 补写恢复用 `turn_end` 前没有先恢复 `next_seq`，导致补写 Fact 从 1 开始，可能产生重复序号。

修复后：

- 恢复文件始终以 `O_APPEND` 打开；
- 先恢复 `next_seq`、`fact_count` 和 `last_ts`，再补写中断的 Turn；
- 回归测试验证旧 Fact 不被覆盖，恢复 Fact 使用递增序号。

## 2. 并行 Turn

并发测试发现两个 Session 在同一纳秒启动时可能生成相同 `turn_id`。

Agent 现在使用：

```text
t-<unix_nano>-<process_sequence>
```

不同 Session 的 Turn 可以并行运行，不共享 Turn ID。

## 3. Workspace

Workspace Registry 并入 `session` 插件：

- 注册已存在的绝对目录；
- 持久化 Workspace 身份和当前选择；
- Session 创建时绑定 `workspace_id` 与规范路径；
- Session Workspace 创建后不可修改。

Agent 每轮调用 `session.info` 读取 Session Workspace。

Session WebUI 增加 Workspace 选择器：

- 可按路径添加 Workspace；
- 历史列表只显示当前 Workspace 的 Session；
- 没有选中 Workspace 时禁用新建和发送；
- Session Workspace 在 Chat 中只读显示。

## 4. 多会话

Session Runtime 新增打开列表：

- `session.select` 打开并切换会话；
- `session.close` 关闭标签但不取消 Turn；
- 后台打开会话也接收 Stream Command；
- 打开列表和选中会话按 Workspace 隔离；
- 页面刷新可恢复 Workspace 和打开列表；
- `session-chat` 提供标签栏、运行状态点、关闭按钮和按会话输入草稿。

## 5. 内建浏览器验证

使用隔离端口与临时 Session 目录完成：

1. 添加两个 Workspace。
2. 分别创建 Session。
3. 验证切换 Workspace 后历史列表只显示所属 Session。
4. 同时打开两个标签。
5. 切换标签并验证当前 Workspace 跟随。
6. 关闭标签并验证邻接选择。
7. 刷新页面并验证 Workspace、标签和选中会话恢复。
8. 重启 Session 插件后检查 JSONL 保留 `seq=1,2,3`，没有覆盖或重复。
9. 验证没有 Workspace 时新建和发送按钮禁用。

当时的冒烟截图没有留在仓库里。上面第 5 节的操作步骤是它的替代记录。

## 6. 未实现

本切片完成时，Web Search、下载、后台 Shell 和 Sandbox UI 尚未实现。相关实现随后随插件裁剪从源码移除，代码保留在 git 历史。
