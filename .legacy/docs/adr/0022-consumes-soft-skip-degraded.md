# consumes 软跳过与 degraded 挂载

**Status: accepted**

`Manifest.consumes` 降为弱校验：缺少 provider 时不再 `serve.Start` fail-loud。Host 打印警告，并将该插件标为 degraded——不写入 Capability registry，`/lp` 可见 degraded，Call 时失败。允许「进程活着但永远跑不了 Turn」以换取高自由度与软失败一致的装配体验。

硬依赖改由 `dependsOn`（插件名闭包）与 Scheme `dependsPlugins` 表达；consumes 仅作声明与诊断。supersedes ADR-0017 中「consumes 未满足则启动失败」的语义（软失败原则保留，暴露时机改为挂载期警告 + 使用期错误）。
