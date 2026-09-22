# skill-manager

Skill 发现与装载：扫描 Workspace 约定目录，向 Context Manager 注册目录段，提供模型可调用的 `load_skill`，并展开用户输入里的 `$skill` 触发。

## 提供

- Capability `skills`：`list {workspace}` / `get {workspace, name}` / `expand {workspace, text}`
- Capability `tools`：`load_skill {name}`
- 输入阶段展开 `$skill`（Skill Trigger，见 CONTEXT.md）

## 目录约定

`<Workspace>/.liteagent/skills/<name>/SKILL.md`

## Manifest

- 非 autostart；由 Agent Scheme `coding` 拉起
