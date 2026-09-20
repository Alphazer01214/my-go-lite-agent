# 02 — /refresh 广播 config.reload

**What to build:** commandPlane.refresh 在更新 Manifest 后，对已挂载插件尝试 `config.reload`；无 method 则忽略。帮助文案更新（不热插拔进程，但通知重读配置）。

**Status:** resolved

## Answer

refresh 对 `cp.mounted` 逐个 `config.reload`，错误忽略；文案与 `/help` 已更新。测试 `TestRefreshBroadcastsConfigReload`。

## Comments

（待填）
