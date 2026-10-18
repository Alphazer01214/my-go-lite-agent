# Manifest

`plugin.json` 是插件包的静态注册面。Host 用它发现身份、能力、UI Component 和 Settings Schema。Manifest 不保存运行状态和真实密钥。

插件业务字段和方法契约写在对应插件目录。本文只定义 Host 能理解的结构。

## 1. 顶层字段

```json
{
  "name": "example-plugin",
  "version": "0.1.0",
  "protocol": 1,
  "entry": "example-plugin",
  "description": "Example plugin",
  "autostart": true,
  "priority": 0,
  "provides": ["example"],
  "consumes": [],
  "requires": [],
  "depends_on": [],
  "commands": [],
  "ui": {},
  "settings": null
}
```

| 字段 | 必填 | 含义 |
|---|---|---|
| `name` | 是 | 插件唯一名称，须等于目录名 |
| `version` | 是 | SemVer 版本 |
| `protocol` | 是 | Manifest 契约版本 |
| `entry` | 是 | 可执行入口 |
| `description` | 否 | 人类可读说明 |
| `autostart` | 否 | 是否作为启动根 |
| `priority` | 否 | Consumer 处理顺序，缺省为 `0` |
| `provides` | 否 | 对外能力名 |
| `requires` | 否 | 弱依赖能力名 |
| `consumes` | 否 | 参与流水线的 `(capability, method)` |
| `depends_on` | 否 | 硬依赖插件名 |
| `commands` | 否 | 命令元数据 |
| `ui` | 否 | WebUI 贡献 |
| `settings` | 否 | Settings Schema |

字段名统一使用 `snake_case`。方法名保持 `capability.method` 的半角点格式。

### 加载时机

Host 启动时扫描一次 `plugins/`。运行期间只在用户触发“重新扫描”时再次扫描。

手动扫描不会自动启动新插件。新插件先以 `inactive` 展示，再由插件中心手动启动。

## 2. 能力声明

`provides` 是观测和装配信息。真正路由以运行时 `host.register` 上报的 `(capability, method)` 为准。

`consumes` 声明插件参与的 `(capability, method)` 流水线。`priority` 决定消费顺序。同一 Method 的 Consumer Priority 必须唯一。

`requires` 是弱依赖。缺少能力时插件仍可启动。调用失败返回可恢复错误。

`depends_on` 是硬依赖插件名。Host 用它计算启动顺序。它不参与 Frame 路由。

## 3. UI Schema

```json
{
  "ui": {
    "entry": "ui/main.js",
    "components": [
      {
        "tag": "example-panel",
        "mount_id": "main",
        "slot": "center"
      }
    ]
  }
}
```

| 字段 | 含义 |
|---|---|
| `entry` | 插件 ES Module 入口 |
| `tag` | Custom Element 标签名 |
| `mount_id` | 插件内稳定实例标识 |
| `slot` | v1 插件使用 `left`、`center`、`right` 或 `bottom` |

实例身份固定为：

```text
plugin_name + mount_id
```

校验规则：

- `tag` 以 `<plugin_name>-` 开头；
- `mount_id` 在插件内唯一；
- `slot` 在 v1 主页面内唯一；`top` 由 Host 控件保留；
- 路径为插件包内相对路径；
- 路径不能包含 `..`；
- `ui.entry` 文件必须存在；UI 入口所在目录及其子目录全部允许访问。

v1 不支持 `pages`、`props` 和插件私有路由页面。组件初始业务状态通过 `setData` 传递。

## 4. Settings Schema

```json
{
  "settings": {
    "get": {
      "capability": "example",
      "method": "config.get"
    },
    "save": {
      "capability": "example",
      "method": "config.set"
    },
    "schema": [
      {
        "key": "endpoint",
        "label": "Endpoint",
        "type": "string",
        "required": true
      },
      {
        "key": "token",
        "label": "Token",
        "type": "secret",
        "required": true
      },
      {
        "key": "retries",
        "label": "Retries",
        "type": "number"
      }
    ]
  }
}
```

支持的控件类型：

| Type | 控件 | 说明 |
|---|---|---|
| `string` | 单行文本 | 自由文本 |
| `secret` | 密文输入 | 不回显明文 |
| `number` | 数字输入 | 展示提示 |
| `boolean` | 复选框 | 布尔展示 |

`type` 只决定控件。Host 不校验字段值。配置取值由插件私有配置文件决定。

`required` 只决定必填提示。缺少必填值时，Host 不拒绝提交。

保存流程：

```text
编辑表单
→ settings.save
→ Plugin 持久化
→ 下一次 Capability Call 生效
```

配置修改不影响运行中的 Call。该 Call 继续使用开始时的配置快照。保存失败时，Host 保留表单输入。每个插件独立保存。

Secret 不回显明文。插件可以返回 `configured: true` 或脱敏值。空 secret 输入的含义由插件定义，但必须在插件文档中写明。

Settings 方法使用现有 Frame 传输协议。`settings.get` 和 `settings.save` 只是 Manifest 中的调用地址，不新增传输层。

`settings.get` 的 Request payload 为空对象，Response payload 使用 `values` 包络：

```json
{
  "values": {
    "base_url": "https://api.example.com",
    "model": "default",
    "api_key": {
      "configured": true
    }
  }
}
```

`settings.save` 的 Request payload 使用同一个包络：

```json
{
  "values": {
    "base_url": "https://api.example.com",
    "api_key": "new-secret"
  }
}
```

`settings.save` 的 Response payload 为：

```json
{
  "saved": true
}
```

`values` 的字段值由插件解释。Secret 可以返回脱敏值或配置状态。Host 不读取插件私有配置文件。

## 5. Commands

```json
{
  "commands": [
    {
      "name": "run",
      "description": "Run an example operation",
      "usage": "/example run"
    }
  ]
}
```

Commands 只是发现和帮助信息。执行仍通过 `(capability, method)`。

## 6. 插件示例

具体插件的 Manifest 示例放在 `plugins/<name>/README.md`。本文不把某个插件的能力写成 Host 默认能力。

## 7. 实现状态

`internal/plugin/manifest.go` 已使用 `UIManifest/UIComponent` 和 `SettingsManifest/SettingsField`。Mount Registry、插件资源服务和 Settings Bootstrap 已接入。

改造时应保持：

- Manifest 是唯一静态注册面；
- UI 资源不做 allowlist 或 MIME 白名单限制；
- Host 不读取插件业务配置内容。

Manifest 错误只记录，不中止 Host 或其他插件。

诊断使用统一结构：

```go
type Diagnostic struct {
    Plugin  string `json:"plugin"`
    Path    string `json:"path,omitempty"`
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

`Plugin` 标记插件或平台来源。`Path` 指向 Manifest 和资源位置。`Code` 是稳定诊断码。`Message` 用于显示。诊断不得包含 Secret 明文。

- JSON、字段和路径错误进入诊断列表；
- 未知字段只报告并继续解析；
- UI 和 Settings 错误不影响业务 Capability；
- 无法使用的 Mount 或静态资源被跳过；
- entry 无法解析时，插件保持 `inactive`；
- 进程启动失败时，Provider 进入 `failed`；
- 其他插件继续扫描和启动。
