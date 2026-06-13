# push-gateway

## 当前状态

- 已有 WebSocket notify、ACK 转发、slow session close、resume buffer、Redis route、cross-instance smoke。
- 只做在线唤醒，不拥有 durable inbox。
- 历史缺口通过 delivery-service `PullInbox` 兜底。

## 后续

- Redis route TTL / 故障指标、跨实例 resume 强化、容量测试。
