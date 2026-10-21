# 管理面开发日志

> 状态：已废止
> 取代者：[docs/platform/webui.md](../../platform/webui.md)
> 说明：插件状态枚举、插件中心页面形状与 llm-openai 的 Settings Schema 都已替换。

日期：2026-10-15  
分支：`webui`  
范围：插件状态快照、Plugins 页面、Settings Renderer  
状态：已完成。插件运行时管理进入设计讨论。

## 1. 本次结论

管理面的只读基础和 Settings Renderer 已经落地。

本次完成：

1. 插件状态快照；
2. Plugins 页面；
3. Settings Renderer；
4. Agent 和 llm-openai 的 Settings 接入。

插件启停、禁用名单、重启和崩溃恢复尚未实现。运行时管理需要先统一期望状态、进程状态和依赖状态。

## 2. 工作区状态

Git 工作区仍然未提交。原有 WebUI、Manifest、文档和插件文档改动继续保留。

本次新增或修改：

```text
internal/host/snapshot.go
internal/webui/bootstrap.go
internal/webui/plugin_snapshot.go
internal/webui/static/plugins.mjs
internal/webui/static/settings.mjs
internal/webui/static/app.js
internal/webui/static/index.html
internal/webui/static/shell.css
plugins/agent/settings.go
plugins/llm-openai/settings.go
plugins/agent/plugin.json
plugins/llm-openai/plugin.json
```

开发预览使用 `127.0.0.1:2334`。源码插件缺少可执行文件时，Plugins 页面仍能显示状态，Settings 调用会返回 `plugin_not_mounted`。

## 3. 插件状态快照

`Host.PluginSnapshot()` 输出插件管理面所需的状态数据：

```json
{
  "name": "llm-openai",
  "version": "0.1.0",
  "state": "mounted",
  "enabled": true,
  "provides": ["llm"],
  "depends_on": []
}
```

状态定义：

| 状态 | 含义 |
|---|---|
| `mounted` | 进程已挂载且健康 |
| `degraded` | 进程不健康，或计划启动但启动失败 |
| `available` | 已发现但当前未挂载 |
| `disabled` | 已禁用 |
| `missing` | 被依赖引用但插件不存在 |

`host.plugins` 返回完整快照。WebUI Bootstrap 合并静态 Manifest、UI Mount 和运行状态。快照不包含插件目录和可执行文件路径。

## 4. Plugins 页面

Host Shell 顶栏新增 `Plugins` 入口。

页面展示：

- 名称和版本；
- enabled/disabled；
- mounted/degraded/available/missing；
- Capability 列表；
- UI Mount 列表。

页面当前只读。面板打开时会关闭诊断面板。窄屏下列表改为纵向布局。

## 5. Settings Renderer

Settings 页面读取 `settings.schema`，使用现有 Browser Call 执行 `settings.get` 和 `settings.save`。

支持控件：

| 类型 | 控件 |
|---|---|
| `string` | 单行文本 |
| `secret` | 密文输入 |
| `number` | 数字输入 |
| `boolean` | 复选框 |

行为约束：

- `required` 只作提示，不阻止提交；
- Secret 不回显明文；
- 已配置 Secret 显示“已配置，留空保持不变”；
- 保存失败保留表单输入；
- 保存成功后重新读取配置；
- 每个插件独立保存。

Settings 使用 `values` 包络：

```json
{
  "values": {
    "base_url": "https://api.example.test/v1",
    "model": "default"
  }
}
```

本次没有新增传输协议，也没有开放 Browser Call 的 `host.*` 路由。

## 6. 插件接入

### 6.1 Agent

新增：

```text
agent.settings.get
agent.settings.save
```

Schema：

- `default_scheme`；
- `max_steps`。

Settings 方法继续使用 Agent 现有配置快照和 `config.json` 持久化。

### 6.2 llm-openai

新增：

```text
llm.settings.get
llm.settings.save
```

Schema：

- `base_url`；
- `model`；
- `api_key`。

`api_key` 只返回 `configured` 布尔值。空 Secret 表示不修改。修改从下一次 `llm.complete` 开始生效。

## 7. 设计决策

### 7.1 Bootstrap 是静态管理数据入口

Manifest 负责静态 Schema。Host 负责生命周期快照。WebUI 不读取插件私有配置文件。

### 7.2 Settings 使用 Browser Call

Settings 方法是插件 Capability。浏览器通过 `POST /bridge/call` 调用，不增加第二套插件路由。

### 7.3 required 不做浏览器强校验

Manifest 的 `required` 只提供提示。插件仍负责解释空值和字段语义。

### 7.4 Secret 只传增量

空 Secret 不发送覆盖值。清除 Secret 需要插件后续提供显式操作。

## 8. 验证记录

命令：

```text
go test ./...
go test -race ./internal/host ./internal/webui ./pluginsdk ./plugins/agent ./plugins/llm-openai
node --test internal/webui/loader.test.mjs internal/webui/lite-agent.test.mjs internal/webui/plugins.test.mjs internal/webui/settings.test.mjs
go build ./cmd/liteagent-server
git diff --check
```

结果：

- Go 全量测试通过；
- race 测试通过；
- Node 共 12 项测试通过；
- 构建通过；
- 浏览器实际读取 Agent 和 llm-openai 配置成功；
- API Key 正确显示为未配置；
- Settings、Plugins 面板均通过浏览器验证；
- 控制台无错误。

已知问题：

```text
go vet ./...
plugins/context-manager/main.go:33:
struct field messages has json tag but is not exported
```

该问题早于本次管理面工作。

## 9. 遇到的问题与解决方法

1. **Go embed 在编译时冻结静态资源。**  
   修改 WebUI 后必须重新启动开发服务才能预览。

2. **源码插件目录没有可执行文件。**  
   页面可以渲染，但 Settings Call 返回 `plugin_not_mounted`。开发验证时单独构建 Agent 和 llm-openai。

3. **HTML required 会阻止提交。**  
   Settings Renderer 只显示必填提示，不设置 `required` 属性。

4. **Settings Response 必须保持包络。**  
   `settings.get` 只接受 `values` 对象，不兼容平铺配置响应。

5. **Secret 回显容易泄漏。**  
   `settings.get` 只返回配置状态，表单输入始终从空值开始。

## 10. 非目标

本次没有实现：

- 插件启停；
- 禁用名单持久化；
- 插件重启；
- 崩溃自动恢复；
- 依赖级联；
- 插件安装和卸载；
- 多浏览器状态同步；
- Settings 历史和回滚。

## 11. 下一步

插件运行时管理需要先讨论状态模型、持久化和故障恢复。

交接文档：

[插件运行时管理交接](2026-10-15-plugin-runtime-management-handoff.md)
