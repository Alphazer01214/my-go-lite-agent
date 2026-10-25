# internal/plugin

`internal/plugin` 负责 Manifest 发现与装载顺序。它是纯读模块，不启动进程，不发帧。


## 1. 职责边界

本包负责：

- 读取 `plugin.json` 并解码；
- 校验 Manifest 的可用性；
- 报告未知字段和路径错误；
- 扫描插件目录；
- 计算 autostart 闭包与启动顺序。

本包不负责：

- 启动进程。进程在 `internal/host` 启动；
- 路由。路由表由 `host.register` 写入；
- 校验插件的业务配置。

本包不 import `internal/host`。两个包之间没有循环依赖。

## 2. 文件与职责

| 文件 | 职责 |
|---|---|
| `manifest.go` | Manifest 结构、加载、未知字段诊断、字段类型校验 |
| `discovery.go` | 目录扫描 |
| `mount.go` | autostart 闭包计算 |
| `diagnostic.go` | 诊断结构与诊断码常量 |

## 3. Manifest 结构

```go
type Manifest struct {
    Name        string
    Version     string
    Protocol    int
    Description string
    Autostart   bool
    Entry       string
    Priority    int
    Provides    []string
    Requires    []string
    DependsOn   []string
    Consumes    []ConsumesMethod
    Commands    []Command
    UI          *UIManifest
    Settings    *SettingsManifest
}
```

| 字段 | JSON | 说明 |
|---|---|---|
| `Name` | `name` | 插件身份。须等于目录名 |
| `Version` | `version` | 插件版本 |
| `Protocol` | `protocol` | Manifest 契约版本 |
| `Description` | `description` | 人类可读说明 |
| `Autostart` | `autostart` | 是否作为启动根 |
| `Entry` | `entry` | 可执行入口 |
| `Priority` | `priority` | 流水线顺序。缺省 0 |
| `Provides` | `provides` | 对外能力名 |
| `Requires` | `requires` | 弱依赖能力名 |
| `DependsOn` | `depends_on` | 硬依赖插件名 |
| `Consumes` | `consumes` | 参与流水线的 `(capability, method)` |
| `Commands` | `commands` | 命令元数据 |
| `UI` | `ui` | WebUI 贡献 |
| `Settings` | `settings` | Settings Schema |

### 3.1 嵌套结构

```go
type ConsumesMethod struct {
    Capability string
    Method     string
}

type UIManifest struct {
    Entry      string
    Components []UIComponent
}

type UIComponent struct {
    Tag     string
    MountID string
    Slot    string
}

type SettingsManifest struct {
    Get    SettingsMethod
    Save   SettingsMethod
    Schema []SettingsField
}

type SettingsMethod struct {
    Capability string
    Method     string
}

type SettingsField struct {
    Key         string
    Label       string
    Type        SettingsFieldType
    Required    bool
    Description string
    OptionsFrom string
    OptionValue string
    OptionLabel string
    Options     []SettingsOption
    ItemFields  []SettingsItemField
}

type SettingsOption struct {
    Value string
    Label string
}

type SettingsItemField struct {
    Key         string
    Label       string
    Type        SettingsFieldType
    Required    bool
    Description string
    Min         *float64
    Step        *float64
    Options     []SettingsOption
}

type Command struct {
    Name        string
    Description string
    Usage       string
}
```

`Min` 和 `Step` 是指针。`min: 0` 与不写 `min` 可以区分。

### 3.2 字段类型

```go
type SettingsFieldType string
```

合法取值只有六个：`string`、`secret`、`number`、`boolean`、`select`、`list`。

其它取值会得到一条 `settings_unknown_type` 诊断。Manifest 仍然被解码。

## 4. 加载

`LoadManifest(path)` 返回 `(Manifest, []Diagnostic)`。

### 4.1 步骤

1. 读文件。失败时返回零值 Manifest 和一条 `manifest_read_failed` 诊断。诊断的 `Plugin` 取路径的父目录名。
2. 去掉 UTF-8 BOM。
3. 解成 `map[string]json.RawMessage`。失败或不是对象时返回一条 `manifest_invalid_json` 诊断。
4. 收集顶层未知字段诊断。
5. 递归收集嵌套未知字段与类型诊断。
6. 解码成 `Manifest`。失败时追加一条 `manifest_invalid_json` 诊断。

未知字段永远不阻止加载。Manifest 的可用性由第 5 节的规则决定。

### 4.2 未知字段诊断

每一层都有一张已知字段白名单：

| 层级 | 白名单 | 诊断码 |
|---|---|---|
| 顶层 | name、version、protocol、description、autostart、entry、priority、provides、requires、depends_on、consumes、commands、ui、settings | `manifest_unknown_field` |
| `ui` | entry、components | `ui_unknown_field` |
| `settings` | get、save、schema | `settings_unknown_field` |
| `settings.get`、`settings.save` | capability、method | `settings_method_unknown_field` |
| `settings.schema[]` | key、label、type、required、description、options_from、option_value、option_label、options、item_fields | `settings_field_unknown_field` |
| `settings.schema[].item_fields[]` | key、label、type、required、description、min、step、options | `settings_item_unknown_field` |

诊断的 `Path` 用片段定位到具体字段：

```text
plugin.json
plugin.json#ui
plugin.json#settings
plugin.json#settings.get
plugin.json#settings.schema[0]
plugin.json#settings.schema[0].item_fields[1]
```

`ui.components[]` 的每一项不再往下校验。因此不会产生 `plugin.json#ui.components[0]` 这类路径。

同一层的多个未知字段按字段名排序后逐条报告。

## 5. 可用性

`ScanRoot` 在收集完诊断后判定 Manifest 是否可用。

| 规则 | 违反时 |
|---|---|
| `version` 非空 | 记 `manifest_unusable`，跳过该插件 |
| `protocol` 大于 0 | 记 `manifest_unusable`，跳过该插件 |
| `name` 非空 | 记 `manifest_unusable`，跳过该插件 |
| `entry` 非空 | 记 `manifest_unusable`，跳过该插件 |
| `name` 等于目录名 | 记 `manifest_name_mismatch`，插件仍然加载 |
| `README.md` 存在 | 记 `plugin_readme_missing`，插件仍然加载 |

前两条合并成一条 `manifest_unusable` 诊断，消息是 `version and protocol are required`。后两条合并成消息 `name and entry are required`。

`manifest_unusable` 的插件不进 `Discoveries.Plugins`。其余诊断只报告，插件照常加载。

`protocol` 只需大于 0。Host 不用它与 Frame 版本比较。

## 6. 目录扫描

`ScanRoot(root)` 遍历 `root` 的一级子目录。

```text
root/
  agent/
    plugin.json
    README.md
  session/
    plugin.json
```

对每个子目录：

1. 没有 `plugin.json` 时记 `manifest_missing` 诊断并跳过。
2. 调 `LoadManifest` 加载并收诊断。
3. 判定可用性。不可用时跳过该目录。
4. `name` 与目录名不一致时记 `manifest_name_mismatch`。插件仍然加载。
5. 没有 `README.md` 时记 `plugin_readme_missing`。插件仍然加载。
6. 加入 `Discoveries.Plugins`。

根目录读不了时记一条 `plugin_root_read_failed` 诊断并返回空结果。

单个目录的坏 Manifest 不影响其它目录。扫描继续。

`Discoveries` 结构：

```go
type Discoveries struct {
    Plugins     []Discovery
    Diagnostics []Diagnostic
}

type Discovery struct {
    Dir      string
    Manifest Manifest
}
```

`Dir` 是插件目录的绝对或相对路径，取决于 `root` 的写法。

## 7. autostart 闭包

`AutostartClosure(discs)` 返回 `(ordered []Discovery, warns []string)`。

### 7.1 规则

1. `autostart: true` 的插件是启动根。
2. 对每个根做深度优先后序遍历，`depends_on` 指向的插件先输出。
3. `depends_on` 指向 `autostart: false` 的插件时，那个插件也被拉进闭包。
4. `autostart: false` 且不是任何人的依赖的插件不进闭包。
5. `depends_on` 的遍历顺序就是 Manifest 里写的顺序。

### 7.2 软失败

| 情形 | 处理 |
|---|---|
| 依赖插件未被发现 | 记 warn，消息含缺失名字和来源。依赖方照常输出 |
| 依赖形成环 | 记 warn，消息含 `cycle` 和栈。环上的插件各输出一次 |
| 同名插件出现多次 | 记 warn，消息含 `duplicate`。先出现的那个胜出 |
| Manifest 名为空 | 记 warn，消息含 `empty manifest name`。该插件被跳过 |

全部是软失败。`ordered` 始终可用，Host 不会因此退出。

环的处理方式：DFS 栈上再次遇到同一个名字时记 warn 并返回，不把该插件输出第二遍。环外的部分正常输出。

## 8. 诊断

```go
type Diagnostic struct {
    Plugin  string `json:"plugin"`
    Path    string `json:"path,omitempty"`
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

`Diagnostic.String()` 输出的格式是：

```text
plugin=<plugin> path=<path> code=<code> message=<message>
```

`Path` 省略时空一列。

### 8.1 诊断码

| 码 | 含义 |
|---|---|
| `plugin_root_read_failed` | 插件根目录读不了 |
| `manifest_missing` | 目录内没有 `plugin.json` |
| `manifest_read_failed` | `plugin.json` 读不了 |
| `manifest_invalid_json` | `plugin.json` 不是合法 JSON 对象 |
| `manifest_unknown_field` | 顶层出现未知字段 |
| `manifest_name_mismatch` | `name` 与目录名不一致 |
| `manifest_unusable` | 缺少必填字段，插件被跳过 |
| `plugin_readme_missing` | 目录内没有 `README.md` |
| `ui_unknown_field` | `ui` 下出现未知字段 |
| `settings_unknown_field` | `settings` 下出现未知字段 |
| `settings_method_unknown_field` | `settings.get` 或 `settings.save` 下出现未知字段 |
| `settings_field_unknown_field` | `settings.schema[]` 条目下出现未知字段 |
| `settings_item_unknown_field` | `settings.schema[].item_fields[]` 条目下出现未知字段 |
| `settings_unknown_type` | 字段类型不在受支持集合内 |

诊断不得包含 Secret 明文。

## 9. 调用方

`internal/host` 使用本包的三处：

| 调用点 | 用途 |
|---|---|
| `host.Start` | `AutostartClosure` 算启动顺序 |
| `host.RescanPlugins` | `ScanRoot` 重新扫描 |
| `host.CreatePluginInstance` | 在发现结果里找插件 |

`internal/webui` 使用 `ScanRoot` 的诊断和 `NewRegistry` 消费 `Discoveries.Plugins`。

## 10. 测试覆盖

| 领域 | 覆盖情况 |
|---|---|
| 未知字段 | 已覆盖顶层与四层嵌套的诊断码。Manifest 仍然解码 |
| 读失败与非法 JSON | 已覆盖。坏 Manifest 不影响其它目录 |
| 名字不匹配 | 已覆盖。只报告，插件仍加载 |
| 必填字段 | 已覆盖。`version` 或 `protocol` 缺失时插件被跳过 |
| UI 与 Settings 解码 | 已覆盖。`min: 0` 与缺失可区分 |
| 字段类型 | 已覆盖。非法类型产生 `settings_unknown_type` |
| 闭包顺序 | 已覆盖依赖先于依赖方、跨 autostart 拉入、非 autostart 排除 |
| 软失败 | 已覆盖缺失依赖、依赖环、同名插件 |

未被测试固定的行为：

- UTF-8 BOM 去除。
- `manifest_missing`、`plugin_root_read_failed`、`plugin_readme_missing`、`settings_unknown_field` 四个诊断码。
- Manifest 名为空的警告。
- 诊断 `Path` 的具体格式。
- 诊断 `Message` 的具体文案。
- `ui.components[]` 条目内的字段校验。当前没有这一层。
