# 插件开发

一个插件是一个目录包。目录名、`plugin.json.name` 和 `pluginsdk.NewPlugin` 的名称必须一致。

目录怎么分、文件怎么拆、什么放哪里，见 [plugin-standard.md](plugin-standard.md)。本文只讲开发流程。

## 1. 包结构

```text
plugins/<name>/
  main.go
  plugin.json
  config.json
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
| `config.json` | 视需要 | 可写运行配置 |
| `errors.go` | 是 | 本插件的错误码常量与构造器 |
| `types.go` | 是 | 跨功能共享的契约类型 |
| `plugin.go` | 是 | plugin 结构体、配置、锁与生命周期 |
| `README.md` | 是 | 功能、配置和方法概览 |
| `communication.md` | 是 | 方法、payload、Event 和前端通信 |
| `ui/` | 视需要 | Web Component 和静态资源 |

业务实现按关注点拆进 `<feature>.go`，规则见 [plugin-standard.md](plugin-standard.md) 第 4 节。

## 2. main.go

```go
package main

import (
    "github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
    p := pluginsdk.NewPlugin("example")
    p.Register("example", "ping", handlePing)
    if err := p.Serve(); err != nil {
        panic(err)
    }
}
```

插件只 import `pluginsdk`。业务代码不手写 Frame 循环。测试夹具可以直接使用 `protocol`。

## 3. 配置边界

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

## 4. 方法文档

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

## 5. 前端组件

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

## 6. 测试

插件测试至少覆盖：

- payload 解码；
- 成功 Response；
- 稳定错误码；
- 配置持久化和脱敏；
- Event payload；
- 纯投影和纯转换函数；
- 取消和错误路径。

## 7. 构建

构建脚本只编译和复制文件。构建脚本不生成 `plugin.json`。

发布目录至少包含：

```text
dist/plugins/<name>/
  <entry>
  plugin.json
  config.json
  README.md
  communication.md
  ui/
```

构建不会写入真实 API Key。已有用户配置在重建时保留。

## 8. 禁止事项

- 插件之间直连；
- 按插件名调用业务能力；
- 在 UI State 中保存插件持久化记录；
- 把临时传输增量直接作为持久记录；
- 让浏览器设置 `from`；
- 在 `plugin.json` 保存 Secret；
- 用 README 替代 `plugin.json` 注册。
