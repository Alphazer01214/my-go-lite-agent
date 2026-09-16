# echo

> 状态：示例 / 参考；在 dist 里保留仅为兼容 CLI / 星型路由的旧调用方式（`-invoke echo`、`consumer` 夹具消费它）。**产品路径上没有任何 scheme 或 autostart 会挂载它。**

最小 Capability Plugin：每个 req 直接以相同 id 回 res。

## 提供

- Capability `echo` / method `echo`：原样回显 payload

## 用途

- Frame 往返、星型路由、生命周期测试的对端；`plugins/consumer` 夹具消费它
