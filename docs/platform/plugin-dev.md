# 插件开发

一个插件是一个目录包。目录名、`plugin.json.name` 和 `pluginsdk.NewPlugin` 的名称必须一致。


## 1. 包结构

```text
plugins/<name>/
  main.go
  plugin.json
  config.json
  config.example.json
  errors.go
  types.go
  plugin.go
  README.md
  communication.md
  ui/
    main.js
    main.css
```

| 文件 | 必填 | 职责 |
|---|---|---|
| `main.go` | 是 | 注册 handler 并启动 Plugin Runtime |
| `plugin.json` | 是 | 静态注册面 |
| `config.json` | 视需要 | 可写运行配置，不提交 Git |
| `config.example.json` | 视需要 | 配置模板，供首次构建落种子 |
| `errors.go` | 是 | 本插件的错误码常量与构造器 |
| `types.go` | 视需要 | 跨功能共享的契约类型 |
| `plugin.go` | 视需要 | plugin 结构体、配置、锁与生命周期 |
| `README.md` | 是 | 功能、配置和方法概览 |
| `communication.md` | 是 | 方法、payload、Event 和前端通信 |
| `ui/` | 视需要 | Web Component 和静态资源 |

`types.go` 和 `plugin.go` 是按需使用的槽位。只有需要共享契约类型或需要集中持有生命周期状态的插件才需要它们。

业务实现按关注点拆进 `<feature>.go`，规则见 [plugin-standard.md](plugin-standard.md) 第 4 节。

`plugin.json` 缺 `version`、`protocol`、`name` 或 `entry` 时，Host 记诊断并跳过该插件。`name` 与目录名不一致时只记诊断，插件仍然加载。

`README.md` 缺失只记诊断，不影响加载。

## 2. main.go

```go
package main

import (
    "github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
    p := pluginsdk.NewPlugin("example")
    p.Register("example", "ping", pluginsdk.NewHandler(handlePing))
    if err := p.Serve(); err != nil {
        panic(err)
    }
}
```

插件只 import `pluginsdk`。业务代码不手写 Frame 循环。测试夹具可以直接使用 `protocol`。

`Register` 的第三个参数是 `pluginsdk.Handler`，必须用 `pluginsdk.NewHandler` 构造。直接传函数会 panic。

`capability` 或 `method` 为空时 `Register` 会 panic。

SDK 的完整调用面见 [pluginsdk.md](pluginsdk.md)。插件不要直接发 `host.*` 调用，那些方法是 SDK 的内部通道。

## 3. 名称一致性

三处名称必须一致：

| 位置 | 作用 |
|---|---|
| 目录名 | Host 扫描时的插件标识 |
| `plugin.json.name` | Manifest 身份 |
| `pluginsdk.NewPlugin` 参数 | 进程身份 |

`pluginsdk.NewPlugin` 的名字会变成出站 Call 的 id 前缀，也会成为 UI Push 的 mount 属主键。名字不一致时，插件发出去的 UI 状态会因为 mount 找不到而被拒。

## 4. 配置边界

`plugin.json` 是静态元数据。`config.json` 是运行配置。

```text
plugin.json  人和 Host 编写
config.json  插件 Runtime 读写
```

真实密钥不进入 `plugin.json`。本地 `config.json` 不提交 Git。仓库只提交 `config.example.json`。

配置更新应满足：

1. 持久化到插件私有配置文件；
2. 返回脱敏结果；
3. 不修改运行中 Call 的配置快照；
4. 从下一次 Call 开始使用新配置。

## 5. 方法文档

每个插件的 `communication.md` 至少包含：

| 章节 | 内容 |
|---|---|
| 能力 | `capability.method` 列表 |
| 方法 payload | JSON 字段和必填性 |
| Response | 返回结构 |
| Event | id、channel、payload |
| 调用关系 | Mermaid 时序图 |
| WebUI | Component、State、Command |
| 错误 | 稳定错误码 |
| 实现状态 | 当前实现与目标契约 |

不要把 Go 私有结构体名当作公共协议。文档使用 JSON 字段名。

## 6. 前端组件

插件 UI 入口定义 Custom Element。标签必须使用插件名前缀。

```js
class ExamplePanel extends HTMLElement {
  setData(state) {
    this.state = state;
    this.render();
  }

  runCommand(command, payload) {
    if (command === "refresh") {
      this.refresh();
    }
  }

  connectedCallback() {
    this.render();
  }

  disconnectedCallback() {
    this.cleanup();
  }
}

customElements.define("example-panel", ExamplePanel);
```

Component 使用 Shadow DOM 隔离样式。Component 不直接读写插件存储，也不建立第二套路由。

可选实现 `resync()`。UI 连接重连时，Shell 优先调用它，失败则移除元素重新挂载。

Component 抛出的异常会中断 UI 流。插件应在自己内部捕获。

`tag` 必须匹配 `^[a-z][a-z0-9._-]*-[a-z0-9._-]+$` 且以 `<plugin_name>-` 开头。`mount_id` 必须匹配 `^[a-z0-9_-]+$`。

## 7. 测试

插件测试至少覆盖：

- payload 解码；
- 成功 Response；
- 稳定错误码；
- 配置持久化和脱敏；
- Event payload；
- 纯投影和纯转换函数；
- 取消和错误路径。

测试文件与被测文件同名，加 `_test.go` 后缀。同一个包的测试用 `package main`。

## 8. 构建

构建脚本只编译和复制文件。构建脚本不生成 `plugin.json`。

发布目录包含：

```text
dist/plugins/<name>/
  <entry>
  plugin.json
  README.md
  ui/                # 有 UI 时
  config.example.json # 有配置时
```

`communication.md` 不进 dist。发行版只带面向用户的 README。

构建不会写入真实 API Key。已有用户配置在重建时保留。

## 9. 禁止事项

- 插件之间直连；
- 按插件名调用业务能力；
- 在 UI State 中保存插件持久化记录；
- 把临时传输增量直接作为持久记录；
- 让浏览器设置 `from`；
- 在 `plugin.json` 保存 Secret；
- 用 README 替代 `plugin.json` 注册。
