# push-gateway

## 当前状态

- 已有 WebSocket notify、ACK 转发、slow session close、resume buffer、Redis route、cross-instance smoke。
- 只做在线唤醒，不拥有 durable inbox。
- 历史缺口通过 delivery-service `PullInbox` 兜底。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏单实例 session / resume 聚合、Redis route / subscriber 聚合和 auth JWK 聚合。
- Redis route 已区分“publish 报错”和“publish 成功但 0 subscriber”两类远端失败，避免把 stale route 误记为远端已入队。

## 后续

- Redis 故障语义、跨实例 resume 强化、容量测试。
