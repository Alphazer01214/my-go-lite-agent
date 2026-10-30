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
| `priority` | 否 | 流水线顺序，缺省为 `0` |
| `provides` | 否 | 对外能力名 |
| `requires` | 否 | 弱依赖能力名 |
| `consumes` | 否 | 参与流水线的 `(capability, method)` |
| `depends_on` | 否 | 硬依赖插件名 |
| `commands` | 否 | 命令元数据 |
| `ui` | 否 | WebUI 贡献 |
| `settings` | 否 | Settings Schema |

字段名统一使用 `snake_case`。方法名保持 `capability.method` 的半角点格式。

未知字段不导致加载失败。Host 记录一条诊断后继续解析其余字段。

### 1.1 加载与校验

Host 扫描 `plugins/` 下的每个子目录。每个子目录必须有 `plugin.json`，否则记一条 `manifest_missing` 诊断并跳过该目录。

校验规则：

| 规则 | 违反时 |
|---|---|
| `version` 非空 | 记 `manifest_unusable`，跳过该插件 |
| `protocol` 大于 0 | 记 `manifest_unusable`，跳过该插件 |
| `name` 非空 | 记 `manifest_unusable`，跳过该插件 |
| `entry` 非空 | 记 `manifest_unusable`，跳过该插件 |
| `name` 等于目录名 | 记 `manifest_name_mismatch`，插件仍然加载 |
| `README.md` 存在 | 记 `plugin_readme_missing`，插件仍然加载 |

`plugin.json` 必须是 JSON 对象。解析失败记 `manifest_invalid_json`。读文件失败记 `manifest_read_failed`。

文件开头的 UTF-8 BOM 会被去掉再解析。

`protocol` 只需大于 0。Host 不用它与 Frame 版本做比较，两者独立演进。

### 1.2 加载时机

Host 启动时扫描一次 `plugins/`。运行期间只在用户触发“重新扫描”时再次扫描。

手动扫描不会自动启动新插件。新插件先以 `inactive` 展示，再由插件中心手动启动。

## 2. 能力声明

`provides` 是观测和装配信息。真正路由以运行时 `host.register` 上报的 `(capability, method)` 为准。

`consumes` 声明插件参与的 `(capability, method)` 流水线。`priority` 决定消费顺序，数值越大越靠前，缺省为 0。同一 Method 的 Consumer Priority 必须唯一。

`requires` 是弱依赖。缺少能力时插件仍可启动，调用失败返回可恢复错误。

`depends_on` 是硬依赖插件名。Host 用它计算启动顺序。它不参与 Frame 路由。

`autostart` 为真时该插件是启动根。Host 对启动根和它们的 `depends_on` 做深度优先后序遍历，保证依赖先于依赖方启动。

缺失依赖与依赖环按软失败处理。Host 记一条启动告警并继续，宿主不退出。

### 2.1 当前未被 Host 消费的字段

有三个字段被解析但没有进入任何运行期路径。

| 字段 | 现状 |
|---|---|
| `requires` | Host 不做能力存在性检查，调用失败由被调方返回错误 |
| `commands` | 不进入插件快照，也不进入 WebUI Bootstrap |
| `description` | 不进入插件快照，也不进入 WebUI Bootstrap |

需要这三项时，由插件在自己的文档和前端界面里承担。

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
- `slot` 可以是 `left`、`center`、`right`、`bottom`；同一插槽允许多个挂载，`top` 由 Host 控件保留；
- 路径为插件包内相对路径；
- 路径不能包含 `..`；
- `ui.entry` 文件必须存在；UI 入口所在目录及其子目录全部允许访问。

格式约束：

| 字段 | 正则 |
|---|---|
| `tag` | `^[a-z][a-z0-9._-]*-[a-z0-9._-]+$`，且以 `<plugin_name>-` 开头 |
| `mount_id` | `^[a-z0-9_-]+$` |

### 3.1 资源根

`ui.entry` 相对插件目录解析。解析结果所在目录成为该插件的资源根。

`/plugins/<plugin>/...` 只暴露资源根下的文件。符号链接解析后仍在资源根内才允许访问，否则返回 404。

`ui.entry` 缺失、不是常规文件、解析到插件包外或目录不可读时，Host 记诊断并跳过该插件的全部 Mount。插件的 Manifest 声明仍然保留。

v1 不支持 `pages`、`props` 和插件私有路由页面。组件初始业务状态通过 `setData` 传递。

## 4. Settings Schema

```json
{
  "settings": {
    "get": {
      "capability": "example",
      "method": "settings.get"
    },
    "save": {
      "capability": "example",
      "method": "settings.save"
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
| `select` | 下拉选择 | 选项来自 `options` 或 `options_from` |
| `list` | 行编辑列表 | `item_fields` 定义每行字段，保存为对象数组 |

`description` 可选，渲染为字段下方的说明文字。

`select` 的选项来源二选一：

- `options`：静态数组，元素为 `{ "value": "...", "label": "..." }`。
- `options_from`：引用同一 Schema 中某个 `list` 字段的 `key`。Host 用该列表当前行生成选项；
  `option_value` 和 `option_label` 指定取值、显示所用字段，默认 `id` / `label`。
  列表行增删或修改时，下拉选项随之更新。

`list` 的 `item_fields` 支持 `string`、`secret`、`number`、`boolean` 与 `select`；数字字段可带 `min`、`step`，
`select` 用 `options` 声明静态选项。Host 不解释字段名，只按行收集成对象数组。完全空白的行不会提交。
`required` 只渲染提示，不阻止提交；业务校验由插件的 `settings.save` 负责。

`list` 与 `select` 的组合示例：

```json
{
  "schema": [
    {
      "key": "models",
      "label": "Models",
      "type": "list",
      "item_fields": [
        { "key": "id", "label": "ID", "type": "string", "required": true },
        { "key": "context_window", "label": "Context window", "type": "number", "min": 0 }
      ]
    },
    {
      "key": "default_model",
      "label": "Default model",
      "type": "select",
      "options_from": "models",
      "option_value": "id",
      "option_label": "id"
    }
  ]
}
```

`type` 只决定控件。Host 不校验字段值。配置取值由插件私有配置文件决定。

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

当前 Host 不消费该字段。见第 2.1 节。

## 6. 插件示例

具体插件的 Manifest 示例放在 `plugins/<name>/README.md`。本文不把某个插件的能力写成 Host 默认能力。

## 7. 实现状态

`internal/plugin/manifest.go` 已使用 `UIManifest/UIComponent` 和 `SettingsManifest/SettingsField`。Mount Registry、插件资源服务和 Settings Bootstrap 已接入。

改造时应保持：

- Manifest 是唯一静态注册面；
- UI 资源不做 allowlist 或 MIME 白名单限制；
- Host 不读取插件业务配置内容。

Manifest 错误只记录，不中止 Host 或其他插件。

## 8. 诊断

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

诊断的处理规则：

- JSON、字段和路径错误进入诊断列表；
- 未知字段只报告并继续解析；
- UI 和 Settings 错误不影响业务 Capability；
- 无法使用的 Mount 或静态资源被跳过；
- entry 无法解析时，插件保持 `inactive`；
- 进程启动失败时，Provider 进入 `failed`；
- 其他插件继续扫描和启动。

### 8.1 诊断码

Host 侧扫描与 Manifest 校验产生的码：

| 码 | 含义 |
|---|---|
| `plugin_root_read_failed` | 插件根目录不可读 |
| `manifest_missing` | 目录内没有 `plugin.json` |
| `manifest_read_failed` | `plugin.json` 读失败 |
| `manifest_invalid_json` | `plugin.json` 不是合法 JSON 对象 |
| `manifest_unknown_field` | Manifest 顶层出现未知字段 |
| `manifest_name_mismatch` | `name` 与目录名不一致 |
| `manifest_unusable` | 缺少必填字段，插件被跳过 |
| `plugin_readme_missing` | 目录内没有 `README.md` |
| `ui_unknown_field` | `ui` 下出现未知字段 |
| `settings_unknown_field` | `settings` 下出现未知字段 |
| `settings_method_unknown_field` | `settings.get` 或 `settings.save` 下出现未知字段 |
| `settings_field_unknown_field` | `settings.schema[]` 条目下出现未知字段 |
| `settings_item_unknown_field` | `settings.schema[].item_fields[]` 条目下出现未知字段 |
| `settings_unknown_type` | 字段类型不在受支持集合内 |

Mount Registry 产生的码：

| 码 | 含义 |
|---|---|
| `ui_plugin_duplicate` | 插件名重复注册 |
| `ui_entry_invalid` | `ui.entry` 不是包内相对路径，或不是常规文件 |
| `ui_entry_missing` | `ui.entry` 指向的文件不存在 |
| `ui_entry_outside_package` | `ui.entry` 解析到插件包外 |
| `ui_resource_root_invalid` | 插件目录不可用，无法确定资源根 |
| `ui_tag_invalid` | `tag` 不是合法 Custom Element 名 |
| `ui_tag_prefix_mismatch` | `tag` 没有以 `<plugin_name>-` 开头 |
| `ui_mount_id_invalid` | `mount_id` 不匹配 `[a-z0-9_-]+` |
| `ui_mount_duplicate` | `mount_id` 在插件内重复 |
| `ui_slot_invalid` | `slot` 不是四个允许值之一 |
| `ui_slot_reserved` | `slot` 用了 Host 保留的 `top` |

浏览器侧产生的码：

| 码 | 含义 |
|---|---|
| `ui_mount_failed` | Mount 装载失败。导入失败、标签定义超时或插槽未知 |
| `ui_resync_failed` | 组件 `resync()` 重试后仍失败 |
| `ui_bootstrap_failed` | Shell 读取 Bootstrap 失败 |

### 8.2 诊断路径记法

诊断的 `Path` 使用带片段的路径定位到具体字段：

```text
plugin.json
plugin.json#ui
plugin.json#ui.components[0]
plugin.json#settings
plugin.json#settings.get
plugin.json#settings.schema[0]
plugin.json#settings.schema[0].item_fields[1]
```

浏览器侧的 Mount 装载诊断把 `Path` 设为 UI 入口 URL，把 `Plugin` 设为插件名。
