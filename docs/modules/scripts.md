# scripts — 构建系统与出厂清单

## 职责

跨平台构建 Host 二进制与出厂插件到 `dist/`，并保持用户配置跨重建不丢。

**不变量**：脚本只编译与拷贝，**永不生成** `plugin.json`（Manifest 真源在插件目录）。

## 文件

| 文件 | 内容 |
|------|------|
| `build.ps1` | Windows PowerShell 构建 |
| `build.sh` | macOS / Linux bash 构建 |
| `shipped-plugins.conf` | 出厂插件清单（唯一真源） |

## shipped-plugins.conf

```text
core: agent session llm-openai context-manager
tools: filetools shelltools sandbox skill-manager project-context webtools
```

- **core**：Autostart 根集相关（核四件；以各插件 `plugin.json` 的 `autostart: true` 为准）
- **tools**：由 Agent Scheme `dependsPlugins` 经 `host.ensurePlugins` 拉起（ADR-0023）
- 注释中还预留了 shipped-but-not-mounted 的 example 组语义，当前清单未列出

## 构建流程（两脚本逻辑一致）

1. 解析 `shipped-plugins.conf`
2. **暂存用户数据**：`llm-openai/config.json`、`agent/config.json`、`session/sessions/`、`config/permissions.json`
3. **增量哈希**：各目标的模块内 `.go`（`go list -deps`）+ `go.mod`/`go.sum`；server 另含 `web/static`；插件另含 `plugin.json`。戳记在 `dist/.build-cache/<target>.sha256`
4. **编译**：`go build -trimpath -ldflags "-s -w"`
   - `cmd/liteagent-cli` → `dist/liteagent-cli(.exe)`
   - `cmd/liteagent-server` → `dist/liteagent-server(.exe)`
5. **安装插件**：有 `entry` 则编 `./plugins/<name>` → `dist/plugins/<name>/`；始终拷贝 `plugin.json`、`ui/`、`README.md`、`segments.json`、`config.example.json`。Unix 上 `sed` 将 entry 从 `<name>.exe` 改为裸名
6. **修剪**未列入清单的 dist 插件目录
7. **恢复/种子配置**：暂存数据回写；llm-openai/agent config 首次从 example 种子
8. **静态资产**：`layout.json`、`sdk/lite-agent.js` → `dist/lite-agent.js`、`README.md`、`CONTEXT.md`、`plugins/README.md`

## 标志

| Windows | Unix | 作用 |
|---------|------|------|
| `-Clean` | `--clean` | 清空 dist 后全量 |
| `-Only name` | `--only` | 仅重建指定目标短名 |
| `-List` | `--list` | 列出可构建目标 |

## dist 布局（摘要）

```text
dist/
  liteagent-cli.exe
  liteagent-server.exe
  lite-agent.js
  layout.json
  README.md  CONTEXT.md
  config/permissions.json
  .build-cache/
  plugins/<name>/{<name>.exe, plugin.json, ui/, config…}
```

运行时：`-plugins dist\plugins`。

## 依赖含义

| 依赖 | 唯一使用点 | 角色 |
|------|------------|------|
| goldmark | `render/mdansi` | Markdown 解析 |
| golang.org/x/term | `internal/app/readline.go` | raw-mode 终端 |
| golang.org/x/sys | 间接 | 终端 syscall |

Host 核心包（serve/protocol/plugin/discovery/assembly/layout）纯 stdlib。

## 开发约定

1. 新增出厂插件：改 `shipped-plugins.conf` + 插件目录；不要让脚本生成 Manifest。
2. 增量哈希漏依赖会导致陈旧二进制——必要时 `-Clean`。
3. 用户 config 路径列表要与插件实际落盘路径保持一致。
4. 构建脚本中出现插件名**允许**（ADR-0030：构建/测试不受 L2 禁令约束）。
