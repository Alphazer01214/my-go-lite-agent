# 插件配置契约（config Capability + /refresh reload）

Status: in-progress

## Problem Statement

插件配置写在磁盘 `config.json`，但运行中进程常在启动时捕获一次（llm-openai `complete` 闭包捕获 cfg）。`/refresh` 只重扫 Manifest，配置改了不生效。Web 设置页需要统一 get/set/schema。

## Solution

- `config` Capability：`get` / `set` / `schema` / `reload`（secret 打码）。
- `/refresh`：Manifest 重扫后对已挂载插件广播 `config.reload`（无则跳过）；仍不热插拔。
- llm-openai：每次 complete 用当前配置；实现 config.*。
- Web Settings UI：另 feature，只依赖本契约。

## Out of Scope

- Settings 页面实现
- 全局配置文件（仍 per-plugin config.json）
