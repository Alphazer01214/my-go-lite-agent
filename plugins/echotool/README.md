# echotool

> 状态：示例 / 参考（不 autostart，也没有任何 scheme 拉起）。需要时把它加进某个 scheme 的 `dependsPlugins`。

最小 Tools Plugin 范例：注册一个 `echo_text` 工具并执行。

## 提供

- Capability `tools`
  - `list` → `echo_text`
  - `call` → `{content, additionalContexts:[{role,content}]}`
- 成功后额外 emit 一张 Presentation Card（`args + result` 的纯投影，无 I/O、无时钟、无随机）

## 用途

- 作为「工具 + Presentation Card + Additional Contexts」写法的最小样例
