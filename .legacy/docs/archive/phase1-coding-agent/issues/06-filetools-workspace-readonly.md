# 06 — filetools：Workspace 根、readOnly schema、越界交 policy

**What to build:** 强化 `plugins/filetools`：

- 所有 path 解析：相对路径相对 call payload 的 `workspace`；缺 workspace 时行为明确（拒绝或仅绝对路径——实现时二选一并写测试）。
- 禁止路径逃逸 `..` 到 Workspace 外的**硬拒绝可保留在插件**作为第二道，但产品裁决仍以 policy 为准；文档写清。
- tools schema 为 `read_file`/`grep`/`glob` 标 `readOnly: true`；`write_file`/`edit_file` 不标。
- 可选：`list_dir` 之类若补工具，只读工具同样打标。
- 更新注释：不再写 sandbox TBD，指向 policy/ADR-0019。

**Blocked by:** 01、02

**Status:** resolved

- [x] workspace 相对解析
- [x] readOnly 标志
- [x] 主缝：根内读写；越界/无 workspace 路径

## Answer

Implemented in Phase 1 commit; host-boundary tests green.

## Comments

- Q18/Q23；与 03 policy 正交。
