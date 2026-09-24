# 插件开发规范 — 包结构

一个插件 = 一个目录包。目录名即插件名，须与 `plugin.json` 的 `name` 一致。Host 的 Discovery 按目录扫描。

名词见 [CONTEXT.md](../CONTEXT.md)，清单字段见 [manifest.md](manifest.md)，进程接口见 [pluginsdk.md](pluginsdk.md)，线协议见 [protocol.md](protocol.md)。

## 包内文件

| 文件 | 必填 | 职责 |
|------|------|------|
| `main.go` | 是 | 进程入口。`package main`，经 `pluginsdk` 注册 handler 并 `Serve()`；**不手写 Frame 循环** |
| `plugin.json` | 是 | Host 注册依据（Manifest）。身份、能力声明、命令与 UI 元数据；Discovery 缺它直接记错并跳过 |
| `config.json` | 视需要 | 本插件的磁盘配置。**唯一可写配置面**；密钥与用户改动不进 git（见仓库 `.gitignore`） |
| `README.md` | 强烈建议 | 面向使用者：插件做什么、对外提供哪些 `capability.method`、怎么配置、有哪些命令。缺失只警告，不阻断挂载 |

可选：`ui/`（前端资产，对应 manifest `ui`）、`config.example.json`（无密钥的配置模板）。

## 布局

```text
plugins/<name>/
  main.go            # 入口
  plugin.json        # Host 注册依据
  config.json        # 本插件配置（可选；通常 gitignore）
  README.md          # 功能与对外方法说明
  ui/                # 可选，Web 组件
```

构建产物（`dist/plugins/<name>/`）至少含：可执行文件（`entry`）、`plugin.json`，以及存在的 `config.json` / `README.md` / `ui/`。构建脚本只编译与拷贝，**不生成** manifest。

## 三文件边界

| 文件 | 谁读 | 谁写 | 含什么 |
|------|------|------|--------|
| `main.go` | 编译器 / 进程 | 插件作者 | handler 与业务逻辑；只 import `pluginsdk` |
| `plugin.json` | Host Discovery / 装配 | 插件作者 | 身份（`name`/`version`/`protocol`/`entry`）、能力（`provides`/`requires`/`depends_on`/`autostart`）、交互（`commands`/`timeout_ms`/`ui`） |
| `config.json` | 插件进程 | 插件作者 / 运行时 `config.set` | 本插件运行参数（模型密钥、scheme、路径等）。Host **不解读**内容 |

- **注册真源是 `plugin.json`**，不是 `main.go` 里的字符串。路由真源是运行时上报的 `(capability, method)`（见 pluginsdk `Serve` 的 `host.register`）。
- **配置不进 `plugin.json`**。清单是静态元数据；可变、可密钥的内容放 `config.json`。
- **README 不参与注册或路由**。它只服务人；`provides` 与 README 里写的方法列表应保持一致。

## README 建议结构

```markdown
# <name>

一句话说明插件做什么。

## 提供的能力
| capability.method | 说明 |
|-------------------|------|
| `foo.bar` | … |

## 配置
`config.json` 字段与环境变量优先级。

## 命令
manifest `commands` 的用法（如 `/foo set k=v`）。
```

## 骨架

```go
// main.go
package main

import "github.com/…/pluginsdk"

func main() {
    p := pluginsdk.NewPlugin("foo")
    p.Register("foo", "bar", handleBar)
    if err := p.Serve(); err != nil {
        panic(err)
    }
}
```

```json
// plugin.json（字段说明见 manifest.md）
{
  "name": "foo",
  "version": "0.1.0",
  "protocol": 1,
  "entry": "foo",
  "provides": ["foo"],
  "description": "…",
  "commands": []
}
```

## 约束

1. 目录名 = `plugin.json` 的 `name` = `pluginsdk.NewPlugin` 的 name。
2. `entry` 指向包内构建出的可执行名（注意 Windows 下常为 `*.exe`）。
3. 插件之间**不直连**；跨能力只 `Call` / `Emit` `(capability, method)`。
4. 生产代码不手写 Frame；测试夹具可触 `protocol`。
5. 核心与 Host 零第三方依赖；插件优先 stdlib。
